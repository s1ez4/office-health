// Пакет update — автообновление из GitHub Releases.
//
// При запуске программа спрашивает GitHub: «какой последний релиз?». Если он новее нашей версии —
// скачивает архив под свою ОС и процессор, сверяет его контрольную сумму SHA-256 со списком
// checksums.txt из того же релиза, подменяет свой файл и перезапускается уже новой версией.
//
// Работу с GitHub, архивами и подменой файла делает библиотека creativeprojects/go-selfupdate.
// На Windows запущенный .exe перезаписать нельзя, поэтому библиотека переименовывает старый файл
// в «.office-health.exe.old» (его можно переименовать даже запущенным), кладёт новый на его место
// и прячет старый — удалится при следующем обновлении.
//
// Нет интернета, GitHub недоступен, любая ошибка — просто работаем дальше старой версией.
package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/creativeprojects/go-selfupdate"
)

// Repo — откуда брать релизы: «владелец/репозиторий» на GitHub.
// Это var, а не const, чтобы при сборке форка можно было подставить свой через -ldflags -X.
var Repo = "s1ez4/office-health"

// SourceEnv — переменная окружения с адресом своего сервера релизов (для проверки обновления без GitHub).
const SourceEnv = "OFFICE_HEALTH_UPDATE_URL"

// Два разных ожидания:
//   - CheckTimeout — сколько ждать ответа «какой последний релиз». Короткое: без сети или при
//     недоступном GitHub программа быстро идёт работать дальше.
//   - DownloadTimeout — сколько ждать скачивания архива. Длинное: на медленном канале 6 МБ с GitHub
//     могут качаться минуту и больше (так и было при первой проверке — 75 секунд).
const (
	CheckTimeout    = 15 * time.Second
	DownloadTimeout = 10 * time.Minute
)

// Notify — куда сообщить «начинаю скачивать». По умолчанию в консоль; nil — молча.
var Notify = func(msg string) { fmt.Println(msg) }

// ErrNoUpdate — обновляться не нужно (уже последняя версия или релизов нет).
var ErrNoUpdate = errors.New("обновлений нет")

// Check — проверить GitHub и обновиться. Возвращает новую версию, если файл программы подменён
// (тогда нужно перезапуститься), или ошибку (в том числе ErrNoUpdate), если всё осталось как было.
func Check(current string) (string, error) {
	var src selfupdate.Source
	var err error
	if url := os.Getenv(SourceEnv); url != "" {
		// Свой сервер вместо GitHub: папка с manifest.yaml и архивами. Нужен, чтобы проверить
		// обновление целиком у себя на компьютере, ничего не выкладывая.
		src, err = selfupdate.NewHttpSource(selfupdate.HttpConfig{BaseURL: url})
	} else {
		src, err = selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	}
	if err != nil {
		return "", err
	}
	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return "", err
	}
	return apply(src, selfupdate.ParseSlug(Repo), current, exe)
}

// apply — вся логика обновления. Источник релизов и путь к файлу — параметры: в тестах подставляем
// поддельный источник и временный файл вместо настоящей программы.
func apply(src selfupdate.Source, repo selfupdate.Repository, current, exePath string) (string, error) {
	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source: src,
		// Сверяем SHA-256 скачанного архива со строчкой в checksums.txt того же релиза.
		// Не совпало (файл испорчен при скачивании или подменён) — обновление отменяется.
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
	})
	if err != nil {
		return "", err
	}
	// context.WithTimeout — «отмени, если не уложились»: по истечении срока запрос обрывается сам.
	checkCtx, cancel := context.WithTimeout(context.Background(), CheckTimeout)
	defer cancel()
	latest, found, err := updater.DetectLatest(checkCtx, repo)
	if err != nil {
		return "", fmt.Errorf("не узнать последний релиз: %w", err)
	}
	if !found || latest.LessOrEqual(current) {
		return "", ErrNoUpdate
	}
	if Notify != nil {
		Notify(fmt.Sprintf("Есть новая версия %s — скачиваю (%.1f МБ)…", latest.Version(), float64(latest.AssetByteSize)/1024/1024))
	}
	dlCtx, cancelDl := context.WithTimeout(context.Background(), DownloadTimeout)
	defer cancelDl()
	if err := updater.UpdateTo(dlCtx, latest, exePath); err != nil {
		return "", fmt.Errorf("не обновиться до %s: %w", latest.Version(), err)
	}
	return latest.Version(), nil
}
