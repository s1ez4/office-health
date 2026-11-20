package update

// Обновление целиком, но без интернета: поддельный «GitHub» (fakeSource) отдаёт релиз с архивом
// и checksums.txt, а вместо настоящей программы подменяется временный файл.

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/creativeprojects/go-selfupdate"
)

type fakeAsset struct {
	id   int64
	name string
	data []byte
}

func (a fakeAsset) GetID() int64                  { return a.id }
func (a fakeAsset) GetName() string               { return a.name }
func (a fakeAsset) GetSize() int                  { return len(a.data) }
func (a fakeAsset) GetBrowserDownloadURL() string { return "https://example.invalid/" + a.name }

type fakeRelease struct {
	tag    string
	assets []fakeAsset
}

func (r fakeRelease) GetID() int64              { return 1 }
func (r fakeRelease) GetTagName() string        { return r.tag }
func (r fakeRelease) GetDraft() bool            { return false }
func (r fakeRelease) GetPrerelease() bool       { return false }
func (r fakeRelease) GetPublishedAt() time.Time { return time.Now() }
func (r fakeRelease) GetReleaseNotes() string   { return "" }
func (r fakeRelease) GetName() string           { return r.tag }
func (r fakeRelease) GetURL() string            { return "" }
func (r fakeRelease) GetAssets() []selfupdate.SourceAsset {
	var out []selfupdate.SourceAsset
	for _, a := range r.assets {
		out = append(out, a)
	}
	return out
}

type fakeSource struct{ rel fakeRelease }

func (s fakeSource) ListReleases(context.Context, selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	return []selfupdate.SourceRelease{s.rel}, nil
}

func (s fakeSource) DownloadReleaseAsset(_ context.Context, _ *selfupdate.Release, id int64) (io.ReadCloser, error) {
	for _, a := range s.rel.assets {
		if a.id == id {
			return io.NopCloser(bytes.NewReader(a.data)), nil
		}
	}
	return nil, errors.New("нет такого файла")
}

func exeName() string {
	if runtime.GOOS == "windows" {
		return "office-health.exe"
	}
	return "office-health"
}

// release собирает релиз, как его делает GoReleaser: архив под нашу ОС + checksums.txt.
func release(t *testing.T, version string, binary []byte, breakChecksum bool) fakeSource {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(exeName())
	w.Write(binary)
	zw.Close()
	name := fmt.Sprintf("office-health_%s_%s_%s.zip", version, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(buf.Bytes())
	if breakChecksum {
		sum[0] ^= 0xFF // «испортили» архив по дороге
	}
	checksums := fmt.Sprintf("%x  %s\n", sum, name)
	return fakeSource{fakeRelease{tag: "v" + version, assets: []fakeAsset{
		{1, name, buf.Bytes()},
		{2, "checksums.txt", []byte(checksums)},
	}}}
}

// installed кладёт «старую версию программы» во временную папку.
func installed(t *testing.T) string {
	path := filepath.Join(t.TempDir(), exeName())
	if err := os.WriteFile(path, []byte("старая версия"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

var repo = selfupdate.ParseSlug("someone/office-health")

func TestUpdates(t *testing.T) {
	exe := installed(t)
	got, err := apply(context.Background(), release(t, "1.1.0", []byte("новая версия"), false), repo, "1.0.0", exe)
	if err != nil || got != "1.1.0" {
		t.Fatalf("обновление: %q %v", got, err)
	}
	if data, _ := os.ReadFile(exe); string(data) != "новая версия" {
		t.Errorf("файл не подменился: %q", data)
	}
}

func TestBadChecksumKeepsOldVersion(t *testing.T) {
	exe := installed(t)
	_, err := apply(context.Background(), release(t, "1.1.0", []byte("подменённая версия"), true), repo, "1.0.0", exe)
	if err == nil {
		t.Fatal("архив с неверной суммой принят")
	}
	if data, _ := os.ReadFile(exe); string(data) != "старая версия" {
		t.Errorf("после отказа файл изменился: %q", data)
	}
}

func TestAlreadyLatest(t *testing.T) {
	exe := installed(t)
	for _, current := range []string{"1.1.0", "1.2.0"} {
		if _, err := apply(context.Background(), release(t, "1.1.0", []byte("x"), false), repo, current, exe); !errors.Is(err, ErrNoUpdate) {
			t.Errorf("версия %s: ждали «обновлений нет», получили %v", current, err)
		}
	}
}
