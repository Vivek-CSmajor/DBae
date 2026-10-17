package page

import (
	"errors"
	"os"
	"sync"
)

var (
	ErrInvalidMagic    = errors.New("magic number validation failed")
	ErrInvalidPageSize = errors.New("page size invalid")
	ErrInvalidPageID   = errors.New("invalid page id")
	ErrDoubleFree      = errors.New("freepage on same page called twice")
)

type PageID uint32
type Page [PageSize]byte //page is raw bytes n array, so no need of structs, cuz structs are combination of diff type of fields
type pageType uint16
type checksum uint32
type lsn uint64

// meta content
type pageCount uint32
type freeListHead uint32

const ( //size ascending
	offType           = 0
	offChecksum       = 2
	offLSN            = 6
	offReserved       = 14
	offMagic          = 24
	offPageSize       = 28
	offPageCount      = 32
	offFreeListHead   = 36
	offNextFreePageID = headerSize
	headerSize        = 24
	// stuff
	PageSize      = 4096 //also meta
	InvalidPageID = 0
	//meta
	metaMagic = 0x44424145 //stands for DBAE in ASCII
)

const (
	pageTypeMeta    pageType = 1
	pageTypeFree    pageType = 2
	pageTypeUnknown pageType = 0
)

type PageManager struct {
	file  *os.File //curr open file
	mutex *sync.Mutex
}
