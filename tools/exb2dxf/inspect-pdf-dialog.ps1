# Read-only Win32 dialog inspection; does not dispatch jobs or click controls.
param(
    [ValidateRange(0, 600)]
    [int]$WatchSeconds = 120,
    [int]$CaxaProcessId = 0,
    [switch]$PrintSelectedPdf
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
if ($CaxaProcessId -eq 0) {
    $processes = @(Get-Process CDRAFT_M -ErrorAction Stop)
    if ($processes.Count -ne 1) { throw 'Multiple CAXA instances; specify -CaxaProcessId.' }
    $CaxaProcessId = $processes[0].Id
} else {
    $process = Get-Process -Id $CaxaProcessId -ErrorAction Stop
    if ($process.ProcessName -ne 'CDRAFT_M') { throw 'The selected process is not CAXA.' }
}

if (-not ('CaxaPdfDialogProbe' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;

public static class CaxaPdfDialogProbe {
    private delegate bool EnumProc(IntPtr hwnd, IntPtr data);
    [DllImport("user32.dll")] private static extern bool EnumWindows(EnumProc callback, IntPtr data);
    [DllImport("user32.dll")] private static extern bool EnumChildWindows(IntPtr hwnd, EnumProc callback, IntPtr data);
    [DllImport("user32.dll")] private static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
    [DllImport("user32.dll")] private static extern bool IsWindowVisible(IntPtr hwnd);
    [DllImport("user32.dll")] private static extern bool IsWindowEnabled(IntPtr hwnd);
    [DllImport("user32.dll")] private static extern int GetDlgCtrlID(IntPtr hwnd);
    [DllImport("user32.dll")] private static extern bool PostMessage(IntPtr hwnd, uint message, IntPtr wp, IntPtr lp);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern int GetClassName(IntPtr hwnd, StringBuilder text, int length);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] private static extern IntPtr SendMessageTimeout(IntPtr hwnd, uint msg, UIntPtr wp, StringBuilder lp, uint flags, uint timeout, out UIntPtr result);

    public class Control {
        public long Handle;
        public int Id;
        public string Class;
        public string Text;
        public bool Visible;
        public bool Enabled;
        public List<Control> Children;
    }

    private static string ClassName(IntPtr hwnd) {
        var text = new StringBuilder(256);
        GetClassName(hwnd, text, text.Capacity);
        return text.ToString();
    }

    private static Control Read(IntPtr hwnd) {
        var text = new StringBuilder(1024);
        UIntPtr result;
        // WM_GETTEXT is marshalled by Windows across processes. Bound hung controls.
        SendMessageTimeout(hwnd, 0x000D, (UIntPtr)text.Capacity, text, 2, 100, out result);
        return new Control {
            Handle = hwnd.ToInt64(), Id = GetDlgCtrlID(hwnd), Class = ClassName(hwnd),
            Text = text.ToString(), Visible = IsWindowVisible(hwnd), Enabled = IsWindowEnabled(hwnd)
        };
    }

    public static List<Control> Snapshot(int processId) {
        var dialogs = new List<Control>();
        EnumWindows(delegate(IntPtr hwnd, IntPtr unused) {
            uint pid;
            GetWindowThreadProcessId(hwnd, out pid);
            if (pid != processId || !IsWindowVisible(hwnd) || ClassName(hwnd) != "#32770") return true;
            var dialog = Read(hwnd);
            dialog.Children = new List<Control>();
            EnumChildWindows(hwnd, delegate(IntPtr child, IntPtr data) {
                dialog.Children.Add(Read(child));
                return true;
            }, IntPtr.Zero);
            dialogs.Add(dialog);
            return true;
        }, IntPtr.Zero);
        return dialogs;
    }

    public static void PrintSelectedPdf(int processId) {
        var matches = new List<Control>();
        foreach (var dialog in Snapshot(processId)) {
            bool pdf = dialog.Children.Exists(c => c.Id == 1136 && c.Class == "ComboBox" && c.Text == "EXB To PDF.drv");
            bool driver = dialog.Children.Exists(c => c.Id == 1098 && c.Text == "CAXA PDF Converter Driver");
            if (pdf && driver && dialog.Enabled) matches.Add(dialog);
        }
        if (matches.Count != 1) throw new InvalidOperationException("Expected exactly one confirmed CAXA PDF print dialog.");
        var button = matches[0].Children.Find(c => c.Id == 1 && c.Class == "Button" && c.Visible && c.Enabled);
        if (button == null) throw new InvalidOperationException("Print button unavailable.");
        // BM_CLICK is asynchronous: printing may enter a modal save dialog.
        if (!PostMessage(new IntPtr(button.Handle), 0x00F5, IntPtr.Zero, IntPtr.Zero))
            throw new InvalidOperationException("Cannot post PDF print click.");
    }
}
'@
}

if ($PrintSelectedPdf) {
    [CaxaPdfDialogProbe]::PrintSelectedPdf($CaxaProcessId)
    Write-Host 'Clicked confirmed EXB To PDF print button; save dialogs will only be inspected.'
    Start-Sleep -Milliseconds 1000
}

$deadline = [DateTime]::UtcNow.AddSeconds($WatchSeconds)
$lastSnapshot = ''
$found = $false
Write-Host "Read-only probe: pid=$CaxaProcessId; watching for $WatchSeconds seconds. Open Print, then the PDF save dialog."
do {
    $null = Get-Process -Id $CaxaProcessId -ErrorAction Stop
    $dialogs = [CaxaPdfDialogProbe]::Snapshot($CaxaProcessId)
    $snapshot = ConvertTo-Json -InputObject @($dialogs.ToArray()) -Depth 5
    if ($dialogs.Count -gt 0 -and $snapshot -ne $lastSnapshot) {
        Write-Host ("--- {0:HH:mm:ss} ---" -f (Get-Date))
        $snapshot
        $found = $true
    }
    $lastSnapshot = $snapshot
    if ([DateTime]::UtcNow -ge $deadline) { break }
    Start-Sleep -Milliseconds 500
} while ($true)
if (-not $found) { Write-Host 'No visible #32770 dialogs found. No changes were made to CAXA.' }
