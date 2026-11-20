//go:build windows

package collect

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf16"
)

type whea struct{}

func (whea) Name() string { return "журнал ошибок железа" }

// wheaScript — PowerShell-скрипт: достать события WHEA-Logger за последние N дней и отдать их в JSON.
// Если событий нет, Get-WinEvent почему-то считает это ошибкой (NoMatchingEventsFound) — ловим и отдаём [].
// %d в тексте заменится на число дней (fmt.Sprintf).
const wheaScript = `
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$ErrorActionPreference = 'Stop'
try {
  $ev = @(Get-WinEvent -FilterHashtable @{
    LogName = 'System'; ProviderName = 'Microsoft-Windows-WHEA-Logger'; StartTime = (Get-Date).AddDays(-%d)
  })
} catch {
  if ($_.FullyQualifiedErrorId -like 'NoMatchingEventsFound*') { $ev = @() }
  else { [Console]::Error.WriteLine($_.Exception.Message); exit 1 }
}
$out = @($ev | ForEach-Object {
  [pscustomobject]@{ Time = $_.TimeCreated.ToUniversalTime().ToString('o'); Id = $_.Id; Message = $_.Message }
})
ConvertTo-Json -InputObject $out -Compress
`

func (whea) Collect(s *Snapshot) error {
	script := fmt.Sprintf(wheaScript, HWErrorDays)
	// -EncodedCommand принимает скрипт в base64 (UTF-16). Так не нужно мучиться с кавычками
	// и русскими буквами в командной строке.
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePS(script))
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 { // errors.As — «это ошибка такого вида?»
			return fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return err
	}
	res, err := parseWinEvents(out)
	if err != nil {
		return err
	}
	s.HWErrors = &res
	return nil
}

// encodePS готовит скрипт для -EncodedCommand: текст → UTF-16 (как хочет Windows) → base64.
func encodePS(script string) string {
	codes := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(codes)*2)
	for _, c := range codes {
		buf = append(buf, byte(c), byte(c>>8)) // младший байт первым (little-endian)
	}
	return base64.StdEncoding.EncodeToString(buf)
}
