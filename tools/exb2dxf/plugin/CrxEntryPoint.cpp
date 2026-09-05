#include "StdAfx.h"

#include <cstdlib>
#include <fstream>
#include <string>
#include <vector>
#include <windows.h>
#include "crxdocman.h"

namespace {

void report(const wchar_t* message, CDraft::ErrorStatus status = CDraft::eOk) {
    crxutPrintf(L"\n[exb2dwg] %s (%d)\n", message, static_cast<int>(status));
}

std::wstring getTempDirectory() {
    wchar_t tempDir[MAX_PATH] = {};
    GetTempPathW(MAX_PATH, tempDir);
    return std::wstring(tempDir);
}

// 识别常见的阻塞弹窗关键字（标题或正文）
static bool containsBlockerKeyword(const std::wstring& text) {
    static const wchar_t* keywords[] = {
        L"\x5f62\x6587\x4ef6",          // 形文件
        L"\x5b57\x4f53",                // 字体
        L"\x6062\x590d",                // 恢复
        L"\x672a\x4fdd\x5b58",          // 未保存
        L"Shape", L"Font", L"Recover", L"Recovery", L"Proxy",
        L"\x4ee3\x7406",                // 代理
        L"\x672a\x627e\x5230",          // 未找到
        L"\x7f3a\x5c11",                // 缺少
    };
    for (const wchar_t* kw : keywords) {
        if (text.find(kw) != std::wstring::npos) return true;
    }
    return false;
}

struct BlockerScanContext {
    bool found;
};

// 扫描对话框子控件文本（部分弹窗标题是通用标题，关键字只出现在正文里）
static BOOL CALLBACK CheckChildTextProc(HWND hwnd, LPARAM lParam) {
    wchar_t text[512] = {};
    GetWindowTextW(hwnd, text, 511);
    if (containsBlockerKeyword(text)) {
        reinterpret_cast<BlockerScanContext*>(lParam)->found = true;
        return FALSE;
    }
    return TRUE;
}

// 自动检测并关闭 CAXA 阻塞弹窗（如“恢复未保存文件”、“指定形文件”、“字体替换”、“代理信息”等）
static BOOL CALLBACK DismissBlockerDialogsProc(HWND hwnd, LPARAM lParam) {
    if (!IsWindowVisible(hwnd)) return TRUE;

    DWORD pid = 0;
    GetWindowThreadProcessId(hwnd, &pid);
    if (pid != GetCurrentProcessId()) return TRUE;

    wchar_t className[256];
    memset(className, 0, sizeof(className));
    GetClassNameW(hwnd, className, 255);

    // 只针对对话框类窗口
    if (wcscmp(className, L"#32770") == 0) {
        wchar_t title[512];
        memset(title, 0, sizeof(title));
        GetWindowTextW(hwnd, title, 511);
        std::wstring titleStr(title);

        // 标题或正文命中关键字都视为阻塞弹窗
        BlockerScanContext ctx = {false};
        if (containsBlockerKeyword(titleStr)) {
            ctx.found = true;
        } else {
            EnumChildWindows(hwnd, CheckChildTextProc, reinterpret_cast<LPARAM>(&ctx));
        }
        if (!ctx.found) return TRUE;

        const std::wstring logPath = getTempDirectory() + L"caxa_worker_log.txt";
        FILE* fp = _wfopen(logPath.c_str(), L"a, ccs=UTF-8");
        if (fp) {
            fwprintf(fp, L"[AutoDismiss] Found blocking dialog '%ls', sending IDCANCEL/ESC\n", title);
            fclose(fp);
        }

        // 优先点“取消”或“忽略”，跳过恢复文档/缺失形文件继续执行
        HWND btnCancel = GetDlgItem(hwnd, IDCANCEL);
        if (btnCancel && IsWindowEnabled(btnCancel)) {
            SendMessage(hwnd, WM_COMMAND, MAKEWPARAM(IDCANCEL, BN_CLICKED), (LPARAM)btnCancel);
        } else {
            SendMessage(hwnd, WM_COMMAND, MAKEWPARAM(IDOK, BN_CLICKED), 0);
        }
        PostMessage(hwnd, WM_CLOSE, 0, 0);
    }
    return TRUE;
}

static void autoDismissModalDialogs() {
    EnumThreadWindows(GetCurrentThreadId(), DismissBlockerDialogsProc, 0);
}

CRxApDocument* g_pendingDocument = nullptr;
std::wstring g_pendingOutput;
ULONGLONG g_pendingSince = 0;
ULONGLONG g_lastSize = 0;
unsigned g_stableSamples = 0;
std::vector<std::pair<std::wstring, std::wstring>> g_tasks;

static void writeCompletion(const std::wstring& path, bool ok) {
    HANDLE file = CreateFileW((path + L".done").c_str(), GENERIC_WRITE, 0,
                             NULL, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
    if (file == INVALID_HANDLE_VALUE) return;
    const char* result = ok ? "OK" : "ERROR";
    DWORD written = 0;
    WriteFile(file, result, static_cast<DWORD>(strlen(result)), &written, NULL);
    CloseHandle(file);
}

// Called once per timer tick; never sleep on the host application's UI thread.
static void pollPendingSave() {
    WIN32_FILE_ATTRIBUTE_DATA info = {};
    bool exists = GetFileAttributesExW(g_pendingOutput.c_str(), GetFileExInfoStandard, &info) != FALSE;
    ULONGLONG size = exists ? (static_cast<ULONGLONG>(info.nFileSizeHigh) << 32) | info.nFileSizeLow : 0;
    g_stableSamples = size > 0 && size == g_lastSize ? g_stableSamples + 1 : 0;
    g_lastSize = size;
    bool stable = g_stableSamples >= 2;
    if (!stable && GetTickCount64() - g_pendingSince < 30000) return;
    auto closeStatus = crxDocManager->closeDocument(g_pendingDocument);
    // An unclosed conversion document must not be followed by another open.
    if (closeStatus != CDraft::eOk) {
        report(L"Cannot close conversion document; queue paused", closeStatus);
        return;
    }
    writeCompletion(g_pendingOutput, stable);
    g_pendingDocument = nullptr;
    g_pendingOutput.clear();
}

bool convertViaAppDoc(const std::wstring& inputPath, const std::wstring& outputPath) {
    const std::wstring logPath = getTempDirectory() + L"caxa_worker_log.txt";

    FILE* fp = _wfopen(logPath.c_str(), L"a, ccs=UTF-8");
    if (fp) {
        fwprintf(fp, L"Attempting appContextOpenDocument: %ls -> %ls\n", inputPath.c_str(), outputPath.c_str());
        fclose(fp);
    }

    CRxApDocument* previousDoc = crxDocManager->curDocument();
    auto status = crxDocManager->appContextOpenDocument(inputPath.c_str());
    if (status != CDraft::eOk) {
        fp = _wfopen(logPath.c_str(), L"a, ccs=UTF-8");
        if (fp) {
            fwprintf(fp, L"appContextOpenDocument failed: %d\n", (int)status);
            fclose(fp);
        }
        return false;
    }

    CRxApDocument* newDoc = crxDocManager->curDocument();
    if (!newDoc || newDoc == previousDoc) {
        report(L"Open did not activate a new document; conversion aborted");
        return false;
    }
    bool saveOk = false;
    if (newDoc && newDoc->database()) {
        std::wstring ext = L"";
        const size_t dotPos = outputPath.find_last_of(L'.');
        if (dotPos != std::wstring::npos) {
            ext = outputPath.substr(dotPos);
        }
        for (size_t i = 0; i < ext.length(); ++i) {
            ext[i] = towlower(ext[i]);
        }

        CDraft::ErrorStatus saveRes = CDraft::eInvalidInput;
        if (ext == L".exb") {
            saveRes = newDoc->database()->saveAs(outputPath.c_str(), false, CRxDb::kEXB_CURRENT);
        } else {
            saveRes = newDoc->database()->saveAs(outputPath.c_str(), false, CRxDb::kDHL_1800);
        }
        saveOk = (saveRes == CDraft::eOk);

        // 验尸日志 1：saveAs 后立刻核对磁盘文件是否真实存在（saveAs 返回 eOk 不代表落盘）
        DWORD attrs1 = GetFileAttributesW(outputPath.c_str());
        bool disk1 = (attrs1 != INVALID_FILE_ATTRIBUTES);
        DWORD err1 = disk1 ? 0 : GetLastError();

        // 验尸日志 2：数据库"认为"自己保存到了哪（排除写到别处的可能）
        const CxCHAR* dbFile = L"(null)";
        newDoc->database()->getFilename(dbFile);

        fp = _wfopen(logPath.c_str(), L"a, ccs=UTF-8");
        if (fp) {
            fwprintf(fp, L"saveAs (%ls) status: %d | disk=%d err=%lu | dbfile=%ls\n",
                     ext.c_str(), (int)saveRes, disk1 ? 1 : 0, err1, dbFile);
            fclose(fp);
        }

        // saveAs 异步落盘：必须等文件非空且稳定后再关闭文档，否则 closeDocument
        // 会取消未完成的写入，留下 0 字节占位文件甚至没有文件。
        if (saveOk) {
            g_pendingDocument = newDoc;
            g_pendingOutput = outputPath;
            g_pendingSince = GetTickCount64();
            g_lastSize = 0;
            g_stableSamples = 0;
            return true;
        }
    } else {
        fp = _wfopen(logPath.c_str(), L"a, ccs=UTF-8");
        if (fp) {
            fwprintf(fp, L"curDocument is NULL!\n");
            fclose(fp);
        }
    }

    if (newDoc) {
        crxDocManager->closeDocument(newDoc);
    }

    return saveOk;
}


UINT_PTR g_timerId = 0;
UINT_PTR g_dialogKillerTimerId = 0;
volatile LONG g_jobBusy = 0;

// 原子认领任务文件：多 CAXA 实例同时轮询时，MoveFileW 只有一个进程能成功，
// 其余实例直接退出。否则两个实例都会拿到同一任务并对同一输入文件
// appContextOpenDocument，第二个实例弹出「文件已打开，是否以只读方式打开」。
static bool claimJobFile(const std::wstring& jobFilePath, std::string& content) {
    wchar_t claimPath[MAX_PATH + 64] = {};
    swprintf_s(claimPath, L"%s.claimed.%lu.%lu", jobFilePath.c_str(),
               static_cast<unsigned long>(GetCurrentProcessId()), GetTickCount());
    if (!MoveFileW(jobFilePath.c_str(), claimPath)) {
        return false;
    }
    HANDLE hFile = CreateFileW(claimPath, GENERIC_READ, FILE_SHARE_READ, NULL, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL);
    if (hFile == INVALID_HANDLE_VALUE) {
        DeleteFileW(claimPath);
        return false;
    }
    DWORD fileSize = GetFileSize(hFile, NULL);
    if (fileSize == 0 || fileSize == INVALID_FILE_SIZE) {
        CloseHandle(hFile);
        DeleteFileW(claimPath);
        return false;
    }
    std::vector<char> buffer(fileSize + 1, 0);
    DWORD bytesRead = 0;
    ReadFile(hFile, buffer.data(), fileSize, &bytesRead, NULL);
    CloseHandle(hFile);
    DeleteFileW(claimPath);
    content.assign(buffer.data(), bytesRead);
    return true;
}

VOID CALLBACK DialogKillerTimerProc(HWND hwnd, UINT uMsg, UINT_PTR idEvent, DWORD dwTime) {
    if (g_jobBusy || g_pendingDocument) autoDismissModalDialogs();
}

VOID CALLBACK MainThreadTimerProc(HWND hwnd, UINT uMsg, UINT_PTR idEvent, DWORD dwTime) {
    // 防重入：转换阻塞期间（模态循环会继续分发 WM_TIMER）不再取新任务
    if (InterlockedCompareExchange(&g_jobBusy, 1, 0) != 0) {
        return;
    }
    struct JobBusyGuard {
        ~JobBusyGuard() { InterlockedExchange(&g_jobBusy, 0); }
    } jobBusyGuard;

    if (g_pendingDocument) {
        pollPendingSave();
        return;
    }

    if (!g_tasks.empty()) {
        const auto task = g_tasks.front();
        g_tasks.erase(g_tasks.begin());
        if (!convertViaAppDoc(task.first, task.second)) writeCompletion(task.second, false);
        return;
    }

    const std::wstring jobFilePath = getTempDirectory() + L"caxa_exb_jobs.txt";
    const std::wstring logPath = getTempDirectory() + L"caxa_worker_log.txt";

    // 原子认领任务文件：rename 成功的进程独占本批任务。
    // 多实例并存时只有认领成功的一方消费任务，其余实例立即退出，
    // 避免两个 CAXA 同时 appContextOpenDocument 弹「文件已打开/只读方式」。
    std::string content;
    if (!claimJobFile(jobFilePath, content)) {
        return;
    }
    if (content.size() >= 3 && (unsigned char)content[0] == 0xEF && (unsigned char)content[1] == 0xBB && (unsigned char)content[2] == 0xBF) {
        content = content.substr(3);
    }
    std::vector<std::pair<std::wstring, std::wstring>> tasks;
    size_t start = 0;
    while (start < content.size()) {
        size_t end = content.find_first_of("\r\n", start);
        if (end == std::string::npos) end = content.size();
        std::string line = content.substr(start, end - start);
        start = (end < content.size() && (content[end] == '\r' || content[end] == '\n')) ? end + 1 : end;
        if (line.empty()) continue;

        size_t sep = line.find("|");
        if (sep != std::string::npos) {
            std::string in_u8 = line.substr(0, sep);
            std::string out_u8 = line.substr(sep + 1);

            int in_len = MultiByteToWideChar(CP_UTF8, 0, in_u8.c_str(), (int)in_u8.size(), NULL, 0);
            std::wstring in_w(in_len, 0);
            MultiByteToWideChar(CP_UTF8, 0, in_u8.c_str(), (int)in_u8.size(), &in_w[0], in_len);

            int out_len = MultiByteToWideChar(CP_UTF8, 0, out_u8.c_str(), (int)out_u8.size(), NULL, 0);
            std::wstring out_w(out_len, 0);
            MultiByteToWideChar(CP_UTF8, 0, out_u8.c_str(), (int)out_u8.size(), &out_w[0], out_len);

            tasks.push_back({in_w, out_w});
        }
    }

    g_tasks = std::move(tasks);
}

void runExb2Dwg() {
}

}  // namespace

class CExb2DwgApp : public AcRxArxApp {
public:
    CExb2DwgApp() : AcRxArxApp() {}

    AcRx::AppRetCode On_kInitAppMsg(void* packet) override {
        const auto result = AcRxArxApp::On_kInitAppMsg(packet);
        crxedRegCmds->addCommand(L"Exb2Dwg", L"GExb2Dwg", L"EXB2DWG", CRX_CMD_MODAL, &runExb2Dwg);
        
        if (g_timerId == 0) {
            g_timerId = SetTimer(NULL, 0, 500, MainThreadTimerProc);
        }
        if (g_dialogKillerTimerId == 0) {
            // 每 200ms 自动巡检并压制/关闭形文件或字体丢失弹窗
            g_dialogKillerTimerId = SetTimer(NULL, 0, 200, DialogKillerTimerProc);
        }

        const std::wstring logPath = getTempDirectory() + L"caxa_worker_log.txt";
        std::wofstream log(logPath, std::ios::app);
        if (log.is_open()) {
            log << L"CExb2DwgApp initialized with MainThread Timer: " << (long long)g_timerId << L"\n";
            log.close();
        }

        return result;
    }

    AcRx::AppRetCode On_kUnloadAppMsg(void* packet) override {
        const auto result = AcRxArxApp::On_kUnloadAppMsg(packet);
        crxedRegCmds->removeGroup(L"Exb2Dwg");
        if (g_timerId != 0) {
            KillTimer(NULL, g_timerId);
            g_timerId = 0;
        }
        if (g_dialogKillerTimerId != 0) {
            KillTimer(NULL, g_dialogKillerTimerId);
            g_dialogKillerTimerId = 0;
        }
        return result;
    }

    void RegisterServerComponents() override {}
};

IMPLEMENT_ARX_ENTRYPOINT(CExb2DwgApp)
