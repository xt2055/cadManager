#include "StdAfx.h"

#include <cstdlib>
#include <fstream>
#include <string>
#include <vector>
#include <windows.h>
#include "crxdocman.h"
#include "crxedcds.h"

namespace {

void report(const wchar_t* message, CDraft::ErrorStatus status = CDraft::eOk) {
    crxutPrintf(L"\n[exb2dwg] %s (%d)\n", message, static_cast<int>(status));
    wchar_t temp[MAX_PATH] = {};
    GetTempPathW(MAX_PATH, temp);
    FILE* fp = _wfopen((std::wstring(temp) + L"caxa_worker_log.txt").c_str(), L"a, ccs=UTF-8");
    if (fp) {
        SYSTEMTIME now = {};
        GetLocalTime(&now);
        fwprintf(fp, L"[%04u-%02u-%02u %02u:%02u:%02u] pid=%lu build=20260908-font-startup-v3 %ls status=%d\n",
            now.wYear, now.wMonth, now.wDay, now.wHour, now.wMinute, now.wSecond,
            GetCurrentProcessId(), message, static_cast<int>(status));
        fclose(fp);
    }
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

// CAXA 的形文件选择框使用自定义按钮 ID，不能假定 IDCANCEL 有效。
static BOOL CALLBACK FindCancelAllButton(HWND hwnd, LPARAM lParam) {
    wchar_t text[128] = {};
    GetWindowTextW(hwnd, text, 127);
    if (IsWindowVisible(hwnd) && IsWindowEnabled(hwnd) &&
        (wcsstr(text, L"\x5168\x90e8\x53d6\x6d88") || wcsstr(text, L"Cancel All"))) {
        *reinterpret_cast<HWND*>(lParam) = hwnd;
        return FALSE;
    }
    return TRUE;
}

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

        // 启动阶段也处理缺失形文件，但其他弹窗仍仅在转换期间处理。
        const bool shapeDialog = titleStr == L"\x6307\x5b9a\x5f62\x6587\x4ef6";
        if (!shapeDialog && !lParam) return TRUE;

        // 标题或正文命中关键字都视为阻塞弹窗
        BlockerScanContext ctx = {false};
        if (containsBlockerKeyword(titleStr)) {
            ctx.found = true;
        } else {
            EnumChildWindows(hwnd, CheckChildTextProc, reinterpret_cast<LPARAM>(&ctx));
        }
        if (!ctx.found) return TRUE;

        HWND cancelAll = nullptr;
        EnumChildWindows(hwnd, FindCancelAllButton, reinterpret_cast<LPARAM>(&cancelAll));
        if (cancelAll) {
            // 直接发送按钮通知，不依赖对话框激活状态；不发送 WM_CLOSE/IDOK。
            const BOOL posted = PostMessageW(GetParent(cancelAll), WM_COMMAND,
                MAKEWPARAM(GetDlgCtrlID(cancelAll), BN_CLICKED), reinterpret_cast<LPARAM>(cancelAll));
            report(posted ? L"Font dialog: Cancel All posted" : L"Font dialog: Cancel All post failed");
            return TRUE;
        }
        if (shapeDialog) return TRUE;

        const std::wstring logPath = getTempDirectory() + L"caxa_worker_log.txt";
        FILE* fp = _wfopen(logPath.c_str(), L"a, ccs=UTF-8");
        if (fp) {
            fwprintf(fp, L"[AutoDismiss] Found blocking dialog '%ls', sending IDCANCEL/ESC\n", title);
            fclose(fp);
        }

        // 优先点“取消”或“忽略”，跳过恢复文档/缺失形文件继续执行
        HWND btnCancel = GetDlgItem(hwnd, IDCANCEL);
        if (btnCancel && IsWindowEnabled(btnCancel)) {
            PostMessageW(btnCancel, BM_CLICK, 0, 0);
        } else {
            PostMessage(hwnd, WM_CLOSE, 0, 0);
        }
    }
    return TRUE;
}

static void autoDismissModalDialogs(bool allowGeneric) {
    // 同进程的弹窗可能属于另一 UI 线程；回调内仍严格校验进程 ID。
    EnumWindows(DismissBlockerDialogsProc, allowGeneric ? 1 : 0);
}

CRxApDocument* g_pendingDocument = nullptr;
std::wstring g_pendingOutput;
/* PDF 功能已废弃，保留原实现供查阅。
bool g_taskIsPdf = false;
// PDF 任务经 sendStringToExecute 触发的打印命令；以 CAXA 命令行实测为准调整。
static const wchar_t* kPdfPrintCommand = L"CX_print\n";
*/
ULONGLONG g_pendingSince = 0;
ULONGLONG g_lastSize = 0;
unsigned g_stableSamples = 0;
ULONGLONG g_lastSampleAt = 0;
ULONGLONG g_closeSince = 0;
bool g_saveOk = false;
bool g_saveStarted = false;
bool g_closeRequested = false;
bool g_failureReported = false;
std::vector<std::pair<std::wstring, std::wstring>> g_tasks;

/* PDF 功能已废弃：禁用对话框探针。
// 阶段0探针：PDF 任务期间把本进程可见对话框及子控件结构 dump 到日志，
// 供实测确认打印/绘图输出对话框的类名、控件 ID 与文本，不做任何自动点击。
static BOOL CALLBACK DialogProbeChildProc(HWND hwnd, LPARAM lParam) {
    auto* out = reinterpret_cast<FILE*>(lParam);
    wchar_t className[256] = {};
    GetClassNameW(hwnd, className, 255);
    wchar_t text[512] = {};
    GetWindowTextW(hwnd, text, 511);
    fwprintf(out, L"    child id=%d class='%ls' text='%ls'\n",
             GetDlgCtrlID(hwnd), className, text);
    return TRUE;
}

static BOOL CALLBACK DialogProbeProc(HWND hwnd, LPARAM lParam) {
    if (!IsWindowVisible(hwnd)) return TRUE;
    DWORD pid = 0;
    GetWindowThreadProcessId(hwnd, &pid);
    if (pid != GetCurrentProcessId()) return TRUE;
    wchar_t className[256] = {};
    GetClassNameW(hwnd, className, 255);

    FILE* out = reinterpret_cast<FILE*>(lParam);
    // 只记录真正的对话框；BCGP 主框架/工具栏全量 dump 会淹没关键信息。
    if (wcscmp(className, L"#32770") == 0) {
        wchar_t title[512] = {};
        GetWindowTextW(hwnd, title, 511);
        fwprintf(out, L"[DialogProbe] hwnd=%p class='%ls' title='%ls'\n",
                 (void*)hwnd, className, title);
        EnumChildWindows(hwnd, DialogProbeChildProc, reinterpret_cast<LPARAM>(out));
    }
    return TRUE;
}

static void dumpDialogProbe() {
    FILE* fp = _wfopen((getTempDirectory() + L"caxa_worker_log.txt").c_str(), L"a, ccs=UTF-8");
    if (!fp) return;
    if (g_pendingDocument) {
        fwprintf(fp, L"[DialogProbe] inputPending=%d quiescent=%d\n",
                 crxDocManager->inputPending(g_pendingDocument),
                 g_pendingDocument->isQuiescent() ? 1 : 0);
    }
    EnumThreadWindows(GetCurrentThreadId(), DialogProbeProc, reinterpret_cast<LPARAM>(fp));
    fclose(fp);
}

*/

static void writeCompletion(const std::wstring& path, bool ok) {
    report(ok ? L"Conversion complete: output stable and document closed" : L"Conversion failed");
    const std::wstring pending = path + L".done.pending";
    HANDLE file = CreateFileW(pending.c_str(), GENERIC_WRITE, 0,
                             NULL, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
    if (file == INVALID_HANDLE_VALUE) return;
    const char* result = ok ? "OK" : "ERROR";
    DWORD written = 0;
    const DWORD length = static_cast<DWORD>(strlen(result));
    const bool complete = WriteFile(file, result, length, &written, NULL) && written == length;
    CloseHandle(file);
    if (!complete || !MoveFileExW(pending.c_str(), (path + L".done").c_str(), MOVEFILE_REPLACE_EXISTING)) {
        report(L"Cannot publish conversion completion");
        DeleteFileW(pending.c_str());
    }
}

// Called once per timer tick; never sleep on the host application's UI thread.
static void pollPendingSave() {
    // closeDocument is asynchronous; never dereference a document already closed by the host/user.
    bool documentExists = false;
    CRxApDocumentIterator* docs = crxDocManager->newAcApDocumentIterator();
    for (; !docs->done(); docs->step()) {
        if (docs->document() == g_pendingDocument) documentExists = true;
    }
    delete docs;
    if (!documentExists) {
        if (!g_failureReported) writeCompletion(g_pendingOutput, g_saveOk && g_closeRequested);
        g_pendingDocument = nullptr;
        g_pendingOutput.clear();
        // g_taskIsPdf = false; // PDF 功能已废弃。
        return;
    }
    const ULONGLONG now = GetTickCount64();
    if (!g_saveStarted) {
        // 部分 CAXA 宿主未实现文档锁；该情况沿用应用上下文保存，不能当作锁冲突重试。
        const auto lockStatus = crxDocManager->lockDocument(g_pendingDocument, CRxAp::kWrite, NULL, NULL, false);
        const bool documentLocked = lockStatus == CDraft::eOk;
        const bool lockUnsupported = lockStatus == CDraft::eNotImplementedYet;
        if (!documentLocked && !lockUnsupported) {
            if (now - g_pendingSince < 30000) return;
            report(L"Timed out acquiring conversion document write lock", lockStatus);
            g_saveStarted = true;
        } else {
            if (lockUnsupported) report(L"Document locking unsupported; saving in application context", lockStatus);
            g_saveStarted = true;
            g_pendingSince = now;
            /* PDF 功能已废弃：禁用打印命令投递。
            if (g_taskIsPdf) {
                // 阶段0：触发打印命令。对话框结构由探针记录，输出路径由实测确认后自动化填写。
                const auto cmdStatus = crxDocManager->sendStringToExecute(
                    g_pendingDocument, kPdfPrintCommand, true, true, true);
                report(L"PDF print command dispatched", cmdStatus);
                if (documentLocked) {
                    const auto unlockStatus = crxDocManager->unlockDocument(g_pendingDocument);
                    if (unlockStatus != CDraft::eOk) report(L"Cannot release conversion document write lock", unlockStatus);
                }
                return;
            }
            */
            CDraft::ErrorStatus status = CDraft::eInvalidInput;
            if (g_pendingDocument->database()) {
                const size_t dot = g_pendingOutput.find_last_of(L'.');
                const bool exb = dot != std::wstring::npos &&
                    _wcsicmp(g_pendingOutput.c_str() + dot, L".exb") == 0;
                if (exb) {
                    status = g_pendingDocument->database()->saveAs(g_pendingOutput.c_str(), false, CRxDb::kEXB_CURRENT);
                } else {
                    status = g_pendingDocument->database()->saveAs(g_pendingOutput.c_str(), false, CRxDb::kDHL_1800);
                }
            }
            if (documentLocked) {
                const auto unlockStatus = crxDocManager->unlockDocument(g_pendingDocument);
                if (unlockStatus != CDraft::eOk) report(L"Cannot release conversion document write lock", unlockStatus);
            }
            g_saveOk = status == CDraft::eOk;
            FILE* fp = _wfopen((getTempDirectory() + L"caxa_worker_log.txt").c_str(), L"a, ccs=UTF-8");
            if (fp) {
                fwprintf(fp, L"[Save] pid=%lu output=%ls status=%d disk=%d\n",
                    GetCurrentProcessId(), g_pendingOutput.c_str(), static_cast<int>(status),
                    GetFileAttributesW(g_pendingOutput.c_str()) != INVALID_FILE_ATTRIBUTES);
                fclose(fp);
            }
            return;
        }
    }
    if (g_closeSince != 0) {
        if (now - g_closeSince >= 10000 && !g_failureReported) {
            report(L"Document close timed out; close the conversion document to resume queue");
            writeCompletion(g_pendingOutput, false);
            g_failureReported = true;
        }
        if (g_closeRequested) return;
    }
    if (now - g_lastSampleAt < 500) return;
    g_lastSampleAt = now;
    WIN32_FILE_ATTRIBUTE_DATA info = {};
    bool exists = GetFileAttributesExW(g_pendingOutput.c_str(), GetFileExInfoStandard, &info) != FALSE;
    ULONGLONG size = exists ? (static_cast<ULONGLONG>(info.nFileSizeHigh) << 32) | info.nFileSizeLow : 0;
    g_stableSamples = size > 0 && size == g_lastSize ? g_stableSamples + 1 : 0;
    g_lastSize = size;
    bool stable = g_stableSamples >= 2;
    /* PDF 功能已废弃：禁用打印输出等待。
    if (g_taskIsPdf) {
        // PDF 完成只能以打印输出文件出现且连续稳定判定；打印提交成功不代表已生成。
        if (stable) {
            g_saveOk = true;
        } else if (now - g_pendingSince < 150000) {
            return;
        } else {
            report(L"PDF print output never appeared or stayed unstable within timeout");
        }
    } else
    */
    {
        if (g_saveOk && !stable && now - g_pendingSince < 30000) return;
        if (g_saveOk && !stable) report(L"Save returned success but output never became nonempty and stable");
        g_saveOk = g_saveOk && stable;
    }
    if (g_closeSince == 0) g_closeSince = now;
    auto closeStatus = crxDocManager->closeDocument(g_pendingDocument);
    // An unclosed conversion document must not be followed by another open.
    if (closeStatus != CDraft::eOk) {
        if (!g_failureReported) report(L"Cannot close conversion document; retrying", closeStatus);
        return;
    }
    g_closeRequested = true;
}

bool convertViaAppDoc(const std::wstring& inputPath, const std::wstring& outputPath) {
    // 拒绝残留 PDF 任务，不能让它落入默认 DWG 保存分支。
    const size_t dot = outputPath.find_last_of(L'.');
    if (dot != std::wstring::npos && _wcsicmp(outputPath.c_str() + dot, L".pdf") == 0) {
        report(L"PDF conversion is disabled", CDraft::eInvalidInput);
        return false;
    }
    const std::wstring logPath = getTempDirectory() + L"caxa_worker_log.txt";

    FILE* fp = _wfopen(logPath.c_str(), L"a, ccs=UTF-8");
    if (fp) {
        fwprintf(fp, L"Attempting appContextOpenDocument: %ls -> %ls\n", inputPath.c_str(), outputPath.c_str());
        fclose(fp);
    }

    const ULONGLONG startedAt = GetTickCount64();
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
    const ULONGLONG openElapsed = GetTickCount64() - startedAt;
    crxutPrintf(L"\n[exb2dwg] open elapsed=%llu ms\n", openElapsed);
    if (!newDoc || newDoc == previousDoc) {
        report(L"Open did not activate a new document; conversion aborted");
        return false;
    }
    g_pendingDocument = newDoc;
    g_pendingOutput = outputPath;
    /* PDF 功能已废弃：禁用任务识别。
    const size_t outputDot = outputPath.find_last_of(L'.');
    g_taskIsPdf = outputDot != std::wstring::npos &&
        _wcsicmp(outputPath.c_str() + outputDot, L".pdf") == 0;
    */
    g_pendingSince = GetTickCount64();
    g_lastSize = 0;
    g_stableSamples = 0;
    g_lastSampleAt = 0;
    g_closeSince = 0;
    g_saveOk = false;
    g_saveStarted = false;
    g_closeRequested = false;
    g_failureReported = false;
    return true;
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
    /* PDF 功能已废弃：恢复原有阻塞弹窗处理。
    if (g_pendingDocument && g_taskIsPdf) {
        // 阶段0：PDF 任务只探测对话框结构，不自动点/关，避免误伤打印对话框。
        static ULONGLONG lastProbe = 0;
        const ULONGLONG now = GetTickCount64();
        if (now - lastProbe >= 2000) {
            lastProbe = now;
            dumpDialogProbe();
        }
        return;
    }
    */
    autoDismissModalDialogs(g_jobBusy || g_pendingDocument);
}

static bool readyForNextDocument() {
    const HWND mainWindow = crxMainWnd();
    const auto document = crxDocManager->curDocument();
    // 模态字体窗口和启动文档尚未结束时，不重入 appContextOpenDocument。
    // CAXA 在 WM_TIMER 回调中可能始终报告非静止，不能以 isQuiescent 阻断调度。
    return mainWindow && IsWindowEnabled(mainWindow) && document;
}

static void processJobs(void*) {
    static bool loggedEntry = false;
    if (!loggedEntry) {
        FILE* fp = _wfopen((getTempDirectory() + L"caxa_worker_log.txt").c_str(), L"a, ccs=UTF-8");
        if (fp) {
            fwprintf(fp, L"[Dispatch] pid=%lu application callback entered\n", GetCurrentProcessId());
            fclose(fp);
        }
        loggedEntry = true;
    }
    struct JobBusyGuard {
        ~JobBusyGuard() { InterlockedExchange(&g_jobBusy, 0); }
    } jobBusyGuard;

    if (g_pendingDocument) {
        pollPendingSave();
        return;
    }

    if (!readyForNextDocument()) return;

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

VOID CALLBACK MainThreadTimerProc(HWND hwnd, UINT uMsg, UINT_PTR idEvent, DWORD dwTime) {
    if (!g_pendingDocument && !readyForNextDocument()) return;
    // Keep the guard held until the application-context callback actually runs.
    if (InterlockedCompareExchange(&g_jobBusy, 1, 0) != 0) return;
    const bool applicationContext = crxDocManager->isApplicationContext();
    static bool loggedDispatch = false;
    if (!loggedDispatch) {
        FILE* fp = _wfopen((getTempDirectory() + L"caxa_worker_log.txt").c_str(), L"a, ccs=UTF-8");
        if (fp) {
            fwprintf(fp, L"[Dispatch] pid=%lu timer entered applicationContext=%d\n",
                     GetCurrentProcessId(), applicationContext ? 1 : 0);
            fclose(fp);
        }
        loggedDispatch = true;
    }
    if (applicationContext) {
        processJobs(nullptr);
    } else {
        crxDocManager->executeInApplicationContext(processJobs, nullptr);
    }
}

void runExb2Dwg() {
}

}  // namespace

class CExb2DwgApp : public AcRxArxApp {
public:
    CExb2DwgApp() : AcRxArxApp() {}

    AcRx::AppRetCode On_kInitAppMsg(void* packet) override {
        const auto result = AcRxArxApp::On_kInitAppMsg(packet);
        report(L"Plugin initialized");
        crxedRegCmds->addCommand(L"Exb2Dwg", L"GExb2Dwg", L"EXB2DWG", CRX_CMD_MODAL, &runExb2Dwg);
        
        if (g_timerId == 0) {
            g_timerId = SetTimer(NULL, 0, 100, MainThreadTimerProc);
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
