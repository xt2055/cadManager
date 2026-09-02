#include <Windows.h>

#include <filesystem>
#include <iostream>
#include <string>

#include "dbmain.h"

namespace fs = std::filesystem;

namespace {

void printUsage() {
    std::wcerr << L"Usage: exb2dxf.exe <input.exb> <output.dwg>\n";
}

int fail(const std::wstring& message, CDraft::ErrorStatus status = CDraft::eInvalidInput) {
    std::wcerr << L"Error: " << message << L" (status " << static_cast<int>(status) << L")\n";
    return 1;
}

}  // namespace

int wmain(int argc, wchar_t* argv[]) {
    if (argc != 3) {
        printUsage();
        return 2;
    }

    std::wcerr << L"[1] start\n";

    const fs::path inputPath = fs::absolute(argv[1]);
    const fs::path outputPath = fs::absolute(argv[2]);
    std::error_code error;

    if (!fs::is_regular_file(inputPath, error)) {
        return fail(L"Input file does not exist: " + inputPath.wstring());
    }
    std::wcerr << L"[2] input found\n";
    if (outputPath.extension() != L".dwg" && outputPath.extension() != L".DWG") {
        return fail(L"The current CRX SDK probe only outputs DWG; use a .dwg extension");
    }
    if (outputPath.has_parent_path()) {
        fs::create_directories(outputPath.parent_path(), error);
        if (error) {
            return fail(L"Cannot create output directory: " + outputPath.parent_path().wstring());
        }
    }

    CRxDbDatabase database(false, true);
    std::wcerr << L"[3] database created\n";
    const auto readStatus = database.readExbFile(inputPath.c_str());
    std::wcerr << L"[4] read returned " << static_cast<int>(readStatus) << L"\n";
    if (readStatus != CDraft::eOk) {
        return fail(L"Failed to read EXB", readStatus);
    }

    const auto saveStatus = database.saveAs(outputPath.c_str(), false, CRxDb::kDHL_CURRENT);
    std::wcerr << L"[5] save returned " << static_cast<int>(saveStatus) << L"\n";
    if (saveStatus != CDraft::eOk) {
        return fail(L"Failed to write DWG", saveStatus);
    }

    std::wcout << L"Conversion succeeded: " << inputPath.wstring() << L" -> " << outputPath.wstring() << L"\n";
    return 0;
}
