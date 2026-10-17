package page

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func mustOpen(t *testing.T, path string) *PageManager {
	t.Helper()
	pm, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return pm
}

func mustClose(t *testing.T, pm *PageManager) {
	t.Helper()
	if err := pm.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func mustAllocate(t *testing.T, pm *PageManager) PageID {
	t.Helper()
	id, err := pm.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	return id
}

func TestOpenCloseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	pm := mustOpen(t, path)
	mustClose(t, pm)

	pm = mustOpen(t, path)
	mustClose(t, pm)
}

func TestAllocateThrice(t *testing.T) {
	pm := mustOpen(t, filepath.Join(t.TempDir(), "test.db"))
	defer pm.Close()

	for want := PageID(1); want <= 3; want++ {
		got := mustAllocate(t, pm)
		if got != want {
			t.Fatalf("Allocate = %d, want %d", got, want)
		}
	}
}

func TestAllocateWriteCloseReopenRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	pm := mustOpen(t, path)

	id := mustAllocate(t, pm)
	var want Page
	for i := range want {
		want[i] = byte(i)
	}
	if err := pm.WritePage(id, &want); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	mustClose(t, pm)

	pm = mustOpen(t, path)
	defer pm.Close()
	var got Page
	if err := pm.ReadPage(id, &got); err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if got != want {
		t.Fatalf("page %d read back after reopen differs from page written", id)
	}
}

func TestAllocateFreeAllocate(t *testing.T) {
	pm := mustOpen(t, filepath.Join(t.TempDir(), "test.db"))
	defer pm.Close()

	first := mustAllocate(t, pm)
	if err := pm.FreePage(first); err != nil {
		t.Fatalf("FreePage: %v", err)
	}
	second := mustAllocate(t, pm)
	if second != first {
		t.Fatalf("Allocate after FreePage = %d, want the freed page %d", second, first)
	}
}

func TestFreeTwice(t *testing.T) {
	pm := mustOpen(t, filepath.Join(t.TempDir(), "test.db"))
	defer pm.Close()

	id := mustAllocate(t, pm)
	if err := pm.FreePage(id); err != nil {
		t.Fatalf("first FreePage: %v", err)
	}
	err := pm.FreePage(id)
	if !errors.Is(err, ErrDoubleFree) {
		t.Fatalf("second FreePage error = %v, want ErrDoubleFree", err)
	}
}

func TestGarbageFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, bytes.Repeat([]byte{0xFF}, 4096), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pm, err := Open(path)
	if !errors.Is(err, ErrInvalidMagic) {
		t.Fatalf("Open error = %v, want ErrInvalidMagic", err)
	}
	if pm != nil {
		t.Fatalf("Open returned a PageManager for a garbage file")
	}
}

func TestFreeListSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	pm := mustOpen(t, path)

	a := mustAllocate(t, pm)
	b := mustAllocate(t, pm)
	c := mustAllocate(t, pm)
	_ = b
	if err := pm.FreePage(a); err != nil {
		t.Fatalf("FreePage(%d): %v", a, err)
	}
	if err := pm.FreePage(c); err != nil {
		t.Fatalf("FreePage(%d): %v", c, err)
	}
	mustClose(t, pm)

	pm = mustOpen(t, path)
	defer pm.Close()
	if got := mustAllocate(t, pm); got != c {
		t.Fatalf("first Allocate after reopen = %d, want %d", got, c)
	}
	if got := mustAllocate(t, pm); got != a {
		t.Fatalf("second Allocate after reopen = %d, want %d", got, a)
	}
}
