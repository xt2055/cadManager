// packer.cpp — cadguanliq offline one-click installer / packer (single file, C++17)
//
// Modes:
//   packer.exe pack <srcDir> [outPath]   pack srcDir into a self-extracting installer
//   packer.exe extract [targetDir]       only unpack payload (for testing)
//   packer.exe install [targetDir]       unpack + full installation
//   packer.exe                           same as install
//
// Payload layout appended to this exe:
//   [u64 fileCount]
//   [entry: u16 pathLen][path utf8][u64 dataLen][data]...
//   [8 bytes magic "TUSHPK01"][8 bytes payloadLen]   (payloadLen = everything before these 16 bytes, relative to payload start)
//
// Build: cl /nologo /O2 /EHsc /utf-8 packer.cpp /Fe:packer.exe advapi32.lib shell32.lib

#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <shellapi.h>

#include <algorithm>
#include <cstdint>
#include <cstring>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <vector>

namespace fs = std::filesystem;

static const char MAGIC[8] = { 'T', 'U', 'S', 'H', 'P', 'K', '0', '1' };

// ---------------- CONFIG (must match install.ps1 defaults) ----------------
static const char* DB_NAME = "cadguanliq";
static const char* DB_USER = "postgres";
static const char* DB_PASSWORD = "cadguanliq2026";
static const int DB_PORT = 5432;
static const char* ADMIN_ACCOUNT = "admin";
static const char* ADMIN_NAME = "Administrator";
static const char* ADMIN_PASSWORD = "admin123456";
static const int APP_PORT = 8080;
// ---------------------------------------------------------------------------

static std::string getSelfPath();

static void fatal(const std::string& message) {
    std::cout << "  [FAIL] " << message << "\n";
    std::cout << "\nPress Enter to exit...";
    std::string dummy;
    std::getline(std::cin, dummy);
    ExitProcess(1);
}

static void step(const std::string& message) { std::cout << "==> " << message << "\n"; }
static void ok(const std::string& message)   { std::cout << "  [OK]   " << message << "\n"; }
static void warn(const std::string& message) { std::cout << "  [WARN] " << message << "\n"; }

static std::string readAllBytes(const fs::path& path) {
    std::ifstream file(path, std::ios::binary);
    if (!file) return "";
    return std::string((std::istreambuf_iterator<char>(file)), std::istreambuf_iterator<char>());
}

static void writeAllBytes(const fs::path& path, const std::string& data) {
    if (!path.parent_path().empty()) fs::create_directories(path.parent_path());
    std::ofstream file(path, std::ios::binary | std::ios::trunc);
    if (!file) fatal("cannot write file: " + path.string());
    file.write(data.data(), (std::streamsize)data.size());
}

static void putU16(std::string& out, uint16_t value) {
    out.push_back((char)(value & 0xFF));
    out.push_back((char)((value >> 8) & 0xFF));
}
static void putU64(std::string& out, uint64_t value) {
    for (int i = 0; i < 8; i++) out.push_back((char)((value >> (8 * i)) & 0xFF));
}
static uint16_t getU16(const std::string& buffer, size_t offset) {
    return (uint16_t)(uint8_t)buffer[offset] | ((uint16_t)(uint8_t)buffer[offset + 1] << 8);
}
static uint64_t getU64(const std::string& buffer, size_t offset) {
    uint64_t value = 0;
    for (int i = 7; i >= 0; i--) value = (value << 8) | (uint8_t)buffer[offset + i];
    return value;
}

// ---------------- PROCESS HELPERS ----------------
static std::string getSelfPath() {
    char buffer[MAX_PATH * 2];
    GetModuleFileNameA(NULL, buffer, sizeof(buffer));
    return std::string(buffer);
}

static int runAndWait(const std::string& exe, const std::string& args, const std::string& workDir, DWORD timeoutMs) {
    STARTUPINFOA si{}; si.cb = sizeof(si);
    PROCESS_INFORMATION pi{};
    std::string cmd = "\"" + exe + "\" " + args;
    std::vector<char> cmdBuf(cmd.begin(), cmd.end());
    cmdBuf.push_back('\0');
    BOOL created = CreateProcessA(NULL, cmdBuf.data(), NULL, NULL, FALSE, 0, NULL,
                                  workDir.empty() ? NULL : workDir.c_str(), &si, &pi);
    if (!created) return -1;
    WaitForSingleObject(pi.hProcess, timeoutMs);
    DWORD code = 1;
    GetExitCodeProcess(pi.hProcess, &code);
    CloseHandle(pi.hThread);
    CloseHandle(pi.hProcess);
    return (int)code;
}

static bool runCapture(const std::string& exe, const std::string& args, std::string& output, DWORD timeoutMs = 90000) {
    SECURITY_ATTRIBUTES sa{ sizeof(sa), NULL, TRUE };
    HANDLE readEnd = NULL, writeEnd = NULL;
    if (!CreatePipe(&readEnd, &writeEnd, &sa, 0)) return false;
    SetHandleInformation(readEnd, HANDLE_FLAG_INHERIT, 0);
    STARTUPINFOA si{}; si.cb = sizeof(si);
    si.dwFlags = STARTF_USESTDHANDLES;
    si.hStdOutput = writeEnd;
    si.hStdError = writeEnd;
    PROCESS_INFORMATION pi{};
    std::string cmd = "\"" + exe + "\" " + args;
    std::vector<char> cmdBuf(cmd.begin(), cmd.end());
    cmdBuf.push_back('\0');
    BOOL created = CreateProcessA(NULL, cmdBuf.data(), NULL, NULL, TRUE, CREATE_NO_WINDOW, NULL, NULL, &si, &pi);
    CloseHandle(writeEnd);
    if (!created) { CloseHandle(readEnd); return false; }
    output.clear();
    char chunk[4096];
    DWORD bytesRead = 0;
    while (ReadFile(readEnd, chunk, sizeof(chunk), &bytesRead, NULL) && bytesRead > 0)
        output.append(chunk, bytesRead);
    CloseHandle(readEnd);
    WaitForSingleObject(pi.hProcess, timeoutMs);
    DWORD code = 1;
    GetExitCodeProcess(pi.hProcess, &code);
    CloseHandle(pi.hThread);
    CloseHandle(pi.hProcess);
    return code == 0;
}

static fs::path writeTempSql(const std::string& sql) {
    fs::path tmp = fs::temp_directory_path() / ("cq_" + std::to_string(GetTickCount()) + ".sql");
    writeAllBytes(tmp, sql + "\n");
    return tmp;
}

static bool runSql(const std::string& psql, const std::string& database, const std::string& sql) {
    fs::path tmp = writeTempSql(sql);
    std::string output;
    bool okResult = runCapture(psql,
        "-h 127.0.0.1 -p " + std::to_string(DB_PORT) + " -U " + DB_USER +
        " -d " + database + " -v ON_ERROR_STOP=1 -f \"" + tmp.string() + "\"", output);
    fs::remove(tmp);
    return okResult;
}

// returns true when the query prints 't'
static bool sqlTrue(const std::string& psql, const std::string& database, const std::string& sql) {
    fs::path tmp = writeTempSql(sql);
    std::string output;
    runCapture(psql,
        "-h 127.0.0.1 -p " + std::to_string(DB_PORT) + " -U " + DB_USER +
        " -d " + database + " -t -A -f \"" + tmp.string() + "\"", output);
    fs::remove(tmp);
    return output.find('t') != std::string::npos;
}

static bool elevateIfNeeded() {
    BOOL isAdmin = FALSE;
    SID_IDENTIFIER_AUTHORITY authority = SECURITY_NT_AUTHORITY;
    PSID adminGroup = NULL;
    if (AllocateAndInitializeSid(&authority, 2, SECURITY_BUILTIN_DOMAIN_RID, DOMAIN_ALIAS_RID_ADMINS,
                                 0, 0, 0, 0, 0, 0, &adminGroup)) {
        CheckTokenMembership(NULL, adminGroup, &isAdmin);
        FreeSid(adminGroup);
    }
    if (isAdmin) return false;
    std::string self = getSelfPath();
    SHELLEXECUTEINFOA info{ sizeof(info) };
    info.lpVerb = "runas";
    info.lpFile = self.c_str();
    info.lpParameters = "install";
    info.nShow = SW_SHOWNORMAL;
    if (!ShellExecuteExA(&info)) fatal("administrator elevation was cancelled");
    ExitProcess(0);
}

// ---------------- PACK ----------------
struct Entry { fs::path path; std::string relative; };

static bool isJunk(const fs::path& path, const std::string& name) {
    static const char* junkExt[] = { ".obj", ".lib", ".exp", ".pch", ".pdb", ".ilk", ".done", ".pyc", ".idb", ".res", ".tmp" };
    std::string ext = path.extension().string();
    for (const char* j : junkExt) if (_stricmp(ext.c_str(), j) == 0) return true;
    if (_stricmp(name.c_str(), "pagefile.sys") == 0) return true;
    if (name == ".git" || name == "node_modules" || name == "logs" || name == "updates" || name == "storage" || name == "data") return true;
    return false;
}

static void collectFiles(const fs::path& dir, const fs::path& base, std::vector<Entry>& out) {
    std::error_code ec;
    for (fs::directory_iterator it(dir, ec), end; it != end; it.increment(ec)) {
        const fs::path& full = it->path();
        std::string name = full.filename().string();
        if (isJunk(full, name)) continue;
        std::error_code isDirEc;
        if (it->is_directory(isDirEc)) collectFiles(full, base, out);
        else out.push_back({ full, fs::relative(full, base).generic_string() });
    }
}

static int modePack(const fs::path& srcDir, const fs::path& outPath) {
    if (!fs::is_directory(srcDir)) fatal("source dir not found: " + srcDir.string());
    std::cout << "==> Packing " << srcDir.string() << "\n";
    std::vector<Entry> files;
    collectFiles(srcDir, srcDir, files);
    std::sort(files.begin(), files.end(), [](const Entry& a, const Entry& b) { return a.relative < b.relative; });

    std::string self = readAllBytes(getSelfPath());
    if (self.empty()) fatal("cannot read own executable");

    std::string body;
    putU64(body, (uint64_t)files.size());
    for (const Entry& entry : files) {
        std::string data = readAllBytes(entry.path);
        std::cout << "  + " << entry.relative << " (" << data.size() << " bytes)\n";
        putU16(body, (uint16_t)entry.relative.size());
        body.append(entry.relative);
        putU64(body, (uint64_t)data.size());
        body.append(data);
    }

    std::string tail = body;                // entries
    tail.append(MAGIC, 8);                  // magic
    putU64(tail, (uint64_t)body.size());    // payloadLen = entries bytes only (installer start = size - 16 - payloadLen)

    std::ofstream out(outPath, std::ios::binary | std::ios::trunc);
    if (!out) fatal("cannot write output: " + outPath.string());
    out.write(self.data(), (std::streamsize)self.size());
    out.write(tail.data(), (std::streamsize)tail.size());
    out.close();
    std::cout << "==> Installer written: " << outPath.string()
              << " (" << ((self.size() + tail.size()) / 1024 / 1024) << " MB, "
              << files.size() << " files)\n";
    return 0;
}

// ---------------- PAYLOAD ACCESS ----------------
static bool loadPayload(std::vector<std::pair<std::string, std::string>>& files) {
    std::string self = readAllBytes(getSelfPath());
    if (self.size() < 24) return false;
    size_t magicAt = self.size() - 16;
    if (memcmp(self.data() + magicAt, MAGIC, 8) != 0) return false;
    uint64_t total = getU64(self, magicAt + 8);
    size_t start = self.size() - 16 - (size_t)total;
    size_t pos = start;
    uint64_t count = getU64(self, pos);
    pos += 8;
    for (uint64_t i = 0; i < count; i++) {
        uint16_t pathLen = getU16(self, pos); pos += 2;
        std::string path = self.substr(pos, pathLen); pos += pathLen;
        uint64_t dataLen = getU64(self, pos); pos += 8;
        std::string data = self.substr(pos, (size_t)dataLen); pos += (size_t)dataLen;
        files.push_back({ path, data });
    }
    return true;
}

// ---------------- INSTALL ----------------
static int modeInstall(const std::string& targetArg, bool extractOnly) {
    std::vector<std::pair<std::string, std::string>> files;
    if (!loadPayload(files)) fatal("no payload found (build with: packer pack <dir>)");
    std::cout << "=============================================\n";
    std::cout << " cadguanliq offline installer\n";
    std::cout << " payload: " << files.size() << " files\n";
    std::cout << "=============================================\n";

    fs::path target = targetArg.empty() ? fs::path("C:\\cadguanliq") : fs::path(targetArg);
    if (!extractOnly) elevateIfNeeded();

    // 1. unpack
    step("Unpacking to " + target.string());
    for (const auto& item : files) {
        fs::path out = target / fs::u8path(item.first);
        writeAllBytes(out, item.second);
    }
    ok(std::to_string(files.size()) + " files extracted");
    if (extractOnly) { std::cout << "extract only, done.\n"; return 0; }

    // 2. PostgreSQL silent install
    const char* pf = getenv("ProgramFiles");
    std::string pgHome = std::string(pf ? pf : "C:\\Program Files") + "\\PostgreSQL\\18";
    std::string psql = pgHome + "\\bin\\psql.exe";
    if (fs::exists(psql)) {
        ok("PostgreSQL already installed (skip)");
        warn("if install fails, check DB password matches: " + std::string(DB_PASSWORD));
    } else {
        fs::path installer;
        for (const auto& item : files) {
            if (item.first.find("postgresql-") == 0 && item.first.size() > 4 &&
                _stricmp(item.first.substr(item.first.size() - 4).c_str(), ".exe") == 0) {
                installer = target / fs::u8path(item.first);
                break;
            }
        }
        if (installer.empty()) fatal("PostgreSQL not installed and installer missing in package");
        step("Silent installing PostgreSQL (1-3 minutes)");
        std::string args = "--mode unattended --unattendedmodeui none --superpassword " + std::string(DB_PASSWORD) +
                           " --serverport " + std::to_string(DB_PORT) +
                           " --prefix \"" + pgHome + "\"" +
                           " --enable-components server,commandlinetools --disable-components pgAdmin,stackbuilder";
        int code = runAndWait(installer.string(), args, target.string(), 10 * 60 * 1000);
        if (code != 0) fatal("PostgreSQL installer exit code " + std::to_string(code));
        if (!fs::exists(psql)) fatal("psql not found after install: " + psql);
        ok("PostgreSQL installed");
    }

    // 3. wait for database
    step("Waiting for PostgreSQL service");
    std::string pgReady = pgHome + "\\bin\\pg_isready.exe";
    bool ready = false;
    for (int i = 0; i < 60; i++) {
        std::string out;
        if (runCapture(pgReady, "-h 127.0.0.1 -p " + std::to_string(DB_PORT) + " -U " + DB_USER, out, 10000) &&
            out.find("accepting") != std::string::npos) { ready = true; break; }
        Sleep(2000);
    }
    if (!ready) fatal("PostgreSQL not ready after 120s");
    ok("PostgreSQL accepting connections");
    SetEnvironmentVariableA("PGPASSWORD", DB_PASSWORD);
    SetEnvironmentVariableA("PGCLIENTENCODING", "UTF8");

    // 4. create database
    step("Creating database");
    if (sqlTrue(psql, "postgres", "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='" + std::string(DB_NAME) + "')"))
        ok("database already exists");
    else if (!runSql(psql, "postgres", "CREATE DATABASE \"" + std::string(DB_NAME) + "\";"))
        fatal("create database failed");
    else ok("database created");

    // 5. migrations
    step("Running migrations");
    if (sqlTrue(psql, DB_NAME, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='users')"))
        ok("schema present (skip)");
    else {
        std::vector<fs::path> migrations;
        std::error_code ec;
        for (fs::directory_iterator it(target / "database" / "migrations", ec), end; it != end; it.increment(ec))
            if (it->path().extension() == ".sql") migrations.push_back(it->path());
        std::sort(migrations.begin(), migrations.end());
        if (migrations.empty()) fatal("no migration files in package");
        for (const auto& file : migrations) {
            if (!runSql(psql, DB_NAME, readAllBytes(file))) fatal("migration failed: " + file.filename().string());
            ok("applied " + file.filename().string());
        }
    }

    // 6. admin account
    step("Creating admin account");
    std::string adminSql = std::string("INSERT INTO users (account, display_name, password_hash, status) VALUES ('") +
                           ADMIN_ACCOUNT + "', '" + ADMIN_NAME + "', crypt('" + ADMIN_PASSWORD + "', gen_salt('bf')), 'active') " +
                           "ON CONFLICT (account) DO NOTHING; " +
                           "INSERT INTO user_roles (user_id, role) SELECT id, 'admin' FROM users WHERE account = '" +
                           ADMIN_ACCOUNT + "' ON CONFLICT DO NOTHING;";
    if (!runSql(psql, DB_NAME, adminSql)) fatal("create admin failed");
    ok(std::string("admin / ") + ADMIN_PASSWORD + " (change after first login)");

    // 7. repair venv
    step("Repairing Python venv");
    fs::path venvCfg = target / ".venv" / "pyvenv.cfg";
    fs::path runtimeExe = target / "runtime" / "python" / "python.exe";
    if (!fs::exists(venvCfg)) warn("no .venv in package (skip)");
    else if (!fs::exists(runtimeExe)) fatal("portable python missing: " + runtimeExe.string());
    else {
        std::string cfg = readAllBytes(venvCfg);
        std::string newHome = "home = " + (target / "runtime" / "python").string();
        size_t pos = cfg.find("home = ");
        if (pos != std::string::npos) {
            size_t end = cfg.find('\n', pos);
            cfg = cfg.substr(0, pos) + newHome + (end == std::string::npos ? "" : cfg.substr(end));
        } else cfg += "\n" + newHome + "\n";
        writeAllBytes(venvCfg, cfg);
        std::string out;
        if (runCapture((target / ".venv" / "Scripts" / "python.exe").string(),
                       "-c \"import olefile, sys; print(olefile.__version__, sys.version_info[0], sys.version_info[1])\"", out, 60000))
            ok("venv ready (olefile " + out + ")");
        else fatal("venv python check failed");
    }

    // 8. .env
    step("Writing .env");
    std::string env =
        std::string("# generated by offline installer\n") +
        "CAD_SERVER_ADDR=:" + std::to_string(APP_PORT) + "\n" +
        "CAD_ALLOWED_ORIGINS=*\n" +
        "CAD_DB_HOST=127.0.0.1\n" +
        "CAD_DB_PORT=" + std::to_string(DB_PORT) + "\n" +
        "CAD_DB_NAME=" + DB_NAME + "\n" +
        "CAD_DB_USER=" + DB_USER + "\n" +
        "CAD_DB_PASSWORD=" + DB_PASSWORD + "\n" +
        "CAD_DB_SSL_MODE=disable\n" +
        "CAD_STORAGE_ROOT=./storage/attachments\n" +
        "CAD_LOG_DIR=./logs\n" +
        "CAD_UPDATES_DIR=./updates\n" +
        "CAD_SMB_ENABLED=false\n";
    writeAllBytes(target / ".env", env);
    ok(".env written");

    // 9. CAXA plugin
    step("Installing CAXA plugin (exb2dwg.crx)");
    std::string programFiles = getenv("ProgramFiles") ? getenv("ProgramFiles") : "C:\\Program Files";
    fs::path caxaBase = fs::path(programFiles) / "CAXA";
    fs::path pluginSrc = target / "tools" / "exb2dxf" / "exb2dwg.crx";
    bool pluginDone = false;
    std::error_code copyEc;
    if (fs::exists(caxaBase) && fs::exists(pluginSrc)) {
        for (fs::directory_iterator it(caxaBase, copyEc), end; it != end; it.increment(copyEc)) {
            fs::path bin64 = it->path() / "Bin64";
            if (fs::is_directory(bin64)) {
                fs::copy_file(pluginSrc, bin64 / "exb2dwg.crx", fs::copy_options::overwrite_existing, copyEc);
                if (!copyEc) {
                    ok("plugin installed to " + bin64.string());
                    pluginDone = true;
                }
            }
        }
    }
    if (!pluginDone)
        warn("CAXA CAD not found - install CAXA CAD, then copy tools\\exb2dxf\\exb2dwg.crx into <CAXA>\\Bin64\\ (or rerun installer)");

    // 10. firewall
    step("Opening firewall port");
    runAndWait("netsh.exe", "advfirewall firewall delete rule name=\"cadguanliq-app\"", "", 15000);
    if (runAndWait("netsh.exe",
                   "advfirewall firewall add rule name=\"cadguanliq-app\" dir=in action=allow protocol=TCP localport=" +
                   std::to_string(APP_PORT), "", 15000) == 0)
        ok("port " + std::to_string(APP_PORT) + " opened for LAN clients");
    else warn("firewall rule failed (LAN access may be blocked)");

    // 11. start app (only kill existing process if it belongs to this target dir)
    step("Starting cadguanliq");
    if (fs::exists(target / "cadguanliq.exe.old")) fs::remove(target / "cadguanliq.exe.old");
    runAndWait((target / "cadguanliq.exe").string(), "", target.string(), 3000);
    ok("started");

    ShellExecuteA(NULL, "open", ("http://127.0.0.1:" + std::to_string(APP_PORT)).c_str(), NULL, NULL, SW_SHOWNORMAL);

    std::cout << "\n=============================================\n";
    std::cout << " INSTALL COMPLETE\n";
    std::cout << " url:      http://127.0.0.1:" << APP_PORT << "\n";
    std::cout << " account:  " << ADMIN_ACCOUNT << " / " << ADMIN_PASSWORD << "\n";
    std::cout << " logs:     " << (target / "logs").string() << "\n";
    std::cout << " remember: change admin password after first login\n";
    std::cout << "=============================================\n";
    std::cout << "\nPress Enter to exit...";
    std::string dummy;
    std::getline(std::cin, dummy);
    return 0;
}

int main(int argc, char** argv) {
    SetConsoleOutputCP(65001);
    std::vector<std::string> args(argv + 1, argv + argc);
    if (!args.empty() && args[0] == "pack") {
        if (args.size() < 2) { std::cout << "usage: packer pack <srcDir> [outPath]\n"; return 1; }
        fs::path src = fs::absolute(fs::u8path(args[1]));
        fs::path out = args.size() >= 3 ? fs::u8path(args[2])
                                        : src.parent_path() / (src.filename().string() + "-installer.exe");
        return modePack(src, out);
    }
    std::string mode = args.empty() ? "install" : args[0];
    std::string target = args.size() >= 2 ? args[1] : "";
    if (mode == "extract") return modeInstall(target, true);
    if (mode == "install") return modeInstall(target, false);
    std::cout << "usage:\n  packer pack <srcDir> [outPath]\n  packer [install|extract] [targetDir]\n";
    return 1;
}
