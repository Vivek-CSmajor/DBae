package page

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
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

func TestFreeAllocateFreeAgain(t *testing.T) {
	pm := mustOpen(t, filepath.Join(t.TempDir(), "test.db"))
	defer pm.Close()

	id := mustAllocate(t, pm)
	if err := pm.FreePage(id); err != nil {
		t.Fatalf("first FreePage: %v", err)
	}
	if again := mustAllocate(t, pm); again != id {
		t.Fatalf("Allocate = %d, want the freed page %d", again, id)
	}
	if err := pm.FreePage(id); err != nil {
		t.Fatalf("FreePage after the page was reused: %v", err)
	}
}

func TestReusedPageIsZeroed(t *testing.T) {
	pm := mustOpen(t, filepath.Join(t.TempDir(), "test.db"))
	defer pm.Close()

	a := mustAllocate(t, pm)
	mustAllocate(t, pm)
	if err := pm.FreePage(a); err != nil {
		t.Fatalf("FreePage: %v", err)
	}
	if got := mustAllocate(t, pm); got != a {
		t.Fatalf("Allocate = %d, want the freed page %d", got, a)
	}

	var got, zero Page
	if err := pm.ReadPage(a, &got); err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if got != zero {
		t.Fatalf("reused page %d is not all zeros", a)
	}
}

func TestAllocateConcurrent(t *testing.T) {
	pm := mustOpen(t, filepath.Join(t.TempDir(), "test.db"))
	defer pm.Close()

	const workers = 50
	type result struct {
		id  PageID
		err error
	}
	results := make(chan result, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := pm.Allocate()
			results <- result{id, err}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[PageID]bool)
	for r := range results {
		if r.err != nil {
			t.Fatalf("Allocate: %v", r.err)
		}
		if seen[r.id] {
			t.Fatalf("page %d was handed out twice", r.id)
		}
		seen[r.id] = true
	}
	for id := PageID(1); id <= workers; id++ {
		if !seen[id] {
			t.Fatalf("page %d was never handed out", id)
		}
	}
}

func TestFreePageZero(t *testing.T) {
	pm := mustOpen(t, filepath.Join(t.TempDir(), "test.db"))
	defer pm.Close()

	err := pm.FreePage(0)
	if !errors.Is(err, ErrInvalidPageID) {
		t.Fatalf("FreePage(0) error = %v, want ErrInvalidPageID", err)
	}
}

func TestFreePagePastEndOfFile(t *testing.T) {
	pm := mustOpen(t, filepath.Join(t.TempDir(), "test.db"))
	defer pm.Close()

	if err := pm.FreePage(99); err == nil {
		t.Fatalf("FreePage(99) on a one-page file returned no error")
	}
}

func TestOpenTruncatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, make([]byte, 100), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pm, err := Open(path)
	if err == nil {
		pm.Close()
		t.Fatalf("Open of a 100-byte file returned no error")
	}
	if pm != nil {
		t.Fatalf("Open returned a PageManager together with an error")
	}
}

func TestOpenExistingEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pm := mustOpen(t, path)
	mustClose(t, pm)

	pm = mustOpen(t, path)
	mustClose(t, pm)
}

func TestOpenWrongPageSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	pm := mustOpen(t, path)
	mustClose(t, pm)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	raw[29] = 0x20
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pm, err = Open(path)
	if !errors.Is(err, ErrInvalidPageSize) {
		t.Fatalf("Open error = %v, want ErrInvalidPageSize", err)
	}
	if pm != nil {
		t.Fatalf("Open returned a PageManager for a file with the wrong page size")
	}
}

func TestPageCountPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	pm := mustOpen(t, path)
	for i := 0; i < 3; i++ {
		mustAllocate(t, pm)
	}

	var meta Page
	if err := pm.ReadPage(0, &meta); err != nil {
		t.Fatalf("ReadPage(0): %v", err)
	}
	if got := meta.GetPageCount(); got != 4 {
		t.Fatalf("page count = %d, want 4", got)
	}
	mustClose(t, pm)

	pm = mustOpen(t, path)
	defer pm.Close()
	if err := pm.ReadPage(0, &meta); err != nil {
		t.Fatalf("ReadPage(0) after reopen: %v", err)
	}
	if got := meta.GetPageCount(); got != 4 {
		t.Fatalf("page count after reopen = %d, want 4", got)
	}
}

func TestSettersGettersRoundTrip(t *testing.T) {
	var p Page

	pt := pageType(7)
	p.SetPageType(&pt)
	if got := p.GetPageType(); got != 7 {
		t.Fatalf("GetPageType = %d, want 7", got)
	}

	l := lsn(1<<40 + 5)
	p.SetLSN(&l)
	if got := p.GetLSN(); got != 1<<40+5 {
		t.Fatalf("GetLSN = %d, want %d", got, uint64(1<<40+5))
	}

	if got := p.GetChecksum(); got != 0 {
		t.Fatalf("GetChecksum on an untouched header = %d, want 0", got)
	}

	var meta Page
	pc := pageCount(9)
	meta.SetPageCount(&pc)
	if got := meta.GetPageCount(); got != 9 {
		t.Fatalf("GetPageCount = %d, want 9", got)
	}
	flh := freeListHead(5)
	meta.SetFreeListHead(&flh)
	if got := meta.GetFreeListHead(); got != 5 {
		t.Fatalf("GetFreeListHead = %d, want 5", got)
	}

	var free Page
	free.SetNextFreePageID(11)
	if got := free.GetNextFreePageID(); got != 11 {
		t.Fatalf("GetNextFreePageID = %d, want 11", got)
	}
}

func TestHeaderFieldsDoNotOverlap(t *testing.T) {
	var p Page

	l := lsn(^uint64(0))
	p.SetLSN(&l)
	if got := p.GetPageType(); got != 0 {
		t.Fatalf("SetLSN changed the page type to %d", got)
	}
	if got := p.GetChecksum(); got != 0 {
		t.Fatalf("SetLSN changed the checksum to %d", got)
	}

	pt := pageType(0xFFFF)
	p.SetPageType(&pt)
	if got := p.GetLSN(); got != lsn(^uint64(0)) {
		t.Fatalf("SetPageType changed the LSN to %d", got)
	}
	if got := p.GetChecksum(); got != 0 {
		t.Fatalf("SetPageType changed the checksum to %d", got)
	}
}

func TestNilSettersChangeNothing(t *testing.T) {
	var p Page
	for i := range p {
		p[i] = byte(i)
	}
	before := p

	p.SetPageType(nil)
	p.SetLSN(nil)
	if p != before {
		t.Fatalf("SetPageType(nil) or SetLSN(nil) changed the page")
	}
}

func TestInitMetaPage(t *testing.T) {
	var p Page
	p.initMetaPage()

	if err := p.Validate(); err != nil {
		t.Fatalf("Validate after initMetaPage: %v", err)
	}
	if got := p.GetPageType(); got != pageTypeMeta {
		t.Fatalf("page type = %d, want meta (%d)", got, pageTypeMeta)
	}
	if got := p.GetPageCount(); got != 1 {
		t.Fatalf("page count = %d, want 1", got)
	}
	if got := p.GetFreeListHead(); got != InvalidPageID {
		t.Fatalf("free list head = %d, want %d (empty)", got, InvalidPageID)
	}
}
