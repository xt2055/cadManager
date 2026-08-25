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

bool convertViaAppDoc(const std::wstring& inputPath, const std::wstring& outputPath) {
    const std::wstring logPath = getTempDirectory() + L"caxa_worker_log.txt";

    FILE* fp = _wfopen(logPath.c_str(), L"a, ccs=UTF-8");
    if (fp) {
        fwprintf(fp, L"Attempting appContextOpenDocument: %ls -> %ls\n", inputPath.c_str(), outputPath.c_str());
        fclose(fp);
    }

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
    bool saveOk = false;
    if (newDoc && newDoc->database()) {
        auto saveRes = newDoc->database()->saveAs(outputPath.c_str(), false, CRxDb::kDHL_1800);
        saveOk = (saveRes == CDraft::eOk);
        fp = _wfopen(logPath.c_str(), L"a, ccs=UTF-8");
        if (fp) {
            fwprintf(fp, L"saveAs status: %d\n", (int)saveRes);
            fclose(fp);
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

VOID CALLBACK MainThreadTimerProc(HWND hwnd, UINT uMsg, UINT_PTR idEvent, DWORD dwTime) {
    const std::wstring jobFilePath = getTempDirectory() + L"caxa_exb_jobs.txt";
    const std::wstring logPath = getTempDirectory() + L"caxa_worker_log.txt";

    // 检查是否有任务文件
    if (GetFileAttributesW(jobFilePath.c_str()) == INVALID_FILE_ATTRIBUTES) {
        return;
    }

    HANDLE hFile = CreateFileW(jobFilePath.c_str(), GENERIC_READ, FILE_SHARE_READ, NULL, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL);
    if (hFile == INVALID_HANDLE_VALUE) {
        return;
    }

    DWORD fileSize = GetFileSize(hFile, NULL);
    if (fileSize == 0 || fileSize == INVALID_FILE_SIZE) {
        CloseHandle(hFile);
        return;
    }

    std::vector<char> buffer(fileSize + 1, 0);
    DWORD bytesRead = 0;
    ReadFile(hFile, buffer.data(), fileSize, &bytesRead, NULL);
    CloseHandle(hFile);
    DeleteFileW(jobFilePath.c_str());

    std::string content(buffer.data(), bytesRead);
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

    for (const auto& task : tasks) {
        bool ok = convertViaAppDoc(task.first, task.second);

        std::wstring donePath = task.second + L".done";
        HANDLE hDone = CreateFileW(donePath.c_str(), GENERIC_WRITE, 0, NULL, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
        if (hDone != INVALID_HANDLE_VALUE) {
            const char* res = ok ? "OK" : "ERROR";
            DWORD written = 0;
            WriteFile(hDone, res, (DWORD)strlen(res), &written, NULL);
            CloseHandle(hDone);
        }
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
        crxedRegCmds->addCommand(L"Exb2Dwg", L"GExb2Dwg", L"EXB2DWG", CRX_CMD_MODAL, &runExb2Dwg);
        
        if (g_timerId == 0) {
            g_timerId = SetTimer(NULL, 0, 500, MainThreadTimerProc);
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
        return result;
    }

    void RegisterServerComponents() override {}
};

IMPLEMENT_ARX_ENTRYPOINT(CExb2DwgApp)
