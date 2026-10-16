package page

import (
	"encoding/binary"
)

// convert const no.s into a byte and write them to a slice
// endians descide which byte goes into first slot neither is better just pick one and stick to it
func (p *Page) SetPageType(t *pageType) {
	if t != nil {
		binary.LittleEndian.PutUint16(p[offType:], uint16(*t))
	}
}
//reason why some of these i didnt implement error handling is become nothing can fail them basically
//these are in memory already so writing them cant go wrong
//disk writes may go bad
func (p *Page) SetLSN(l *lsn) {
	if l != nil {
		binary.LittleEndian.PutUint64(p[offLSN:], uint64(*l))
	}
}

func (p *Page) GetPageType() pageType {
	return pageType((binary.LittleEndian.Uint16(p[offType:])))
}

func (p *Page) GetChecksum() checksum {
	return checksum((binary.LittleEndian.Uint32(p[offChecksum:])))
}

func (p *Page) GetLSN() lsn {
	return lsn((binary.LittleEndian.Uint64(p[offLSN:])))
}

// store totalpagecount,headof freelist
func (p *Page) SetPageCount(pc *pageCount) {
	binary.LittleEndian.PutUint32(p[offPageCount:], uint32(*pc))
}

func (p *Page) SetFreeListHead(flh *freeListHead) {
	binary.LittleEndian.PutUint32(p[offFreeListHead:], uint32(*flh))
}

func (p *Page) GetPageCount() pageCount {
	return pageCount((binary.LittleEndian.Uint32(p[offPageCount:])))
}

func (p *Page) GetNextFreePageID() PageID {
	return PageID((binary.LittleEndian.Uint32(p[offNextFreePageID:])))
}

func (p *Page) SetNextFreePageID(id PageID) {
	binary.LittleEndian.PutUint32(p[offNextFreePageID:], uint32(id))
}

func (p *Page) GetFreeListHead() PageID { //freelisthead returns pageID
	return PageID((binary.LittleEndian.Uint32(p[offFreeListHead:])))
}

func (p *Page) initMetaPage() {
	//set page type to meta
	t := pageTypeMeta
	p.SetPageType(&t)
	//set magic value
	binary.LittleEndian.PutUint32(p[offMagic:], metaMagic)
	binary.LittleEndian.PutUint32(p[offPageSize:], PageSize)
	binary.LittleEndian.PutUint32(p[offPageSize:], PageSize)
	pc := pageCount(1)  //converts 1 to pagecount type
	p.SetPageCount(&pc) //&pc is address of variable with value 1, of type *pageCount
	ipid := freeListHead(InvalidPageID)
	p.SetFreeListHead(&ipid)
}

func (p *Page) Validate() error {
	mn := binary.LittleEndian.Uint32(p[offMagic:])
	if mn != metaMagic {
		return ErrInvalidMagic
	}
	ps := binary.LittleEndian.Uint32(p[offPageSize:])
	if ps != PageSize {
		return ErrInvalidPageSize
	}
	return nil
}

//reads , writes require error because they write to disk
func (pm *PageManager) ReadPage(id PageID, p *Page) error {
	_, err := pm.file.ReadAt(p[:], int64(id)*PageSize) //skip _ cuz read bytes go into the p slice already
	if err != nil {
		return err
	}
	return nil
}

func (pm *PageManager) WritePage(id PageID, p *Page) error {
	//put all 4096 bytes of a page into disk in file
	_, err := pm.file.WriteAt(p[:], int64(id)*PageSize)
	if err != nil {
		return err
	}
	return nil
}

// allocate assumes file is already set up
func (pm *PageManager) Allocate() (PageID, error) {
	var meta Page
	if err := pm.ReadPage(0, &meta); err != nil { //will read the meta page into meta
		return 0, err
	}
	head := meta.GetFreeListHead()
	if head != InvalidPageID { //freelist has a page
		var freePage Page
		if err := pm.ReadPage(head, &freePage); err != nil {
			return 0, err
		}
		next := freePage.GetNextFreePageID()
		x := freeListHead(next)
		meta.SetFreeListHead(&x) //this sets meta head in mem
		if err := pm.WritePage(0, &meta); err != nil { //gotta write it again
			return 0, err
		}
		var blank Page 
		if err := pm.WritePage(head, &blank); err != nil { //gotta write it again
			return 0, err
		}
		return head, nil
	}
	//Freelist is only bw first page and the last page used,after that its just appending pages
	pc := meta.GetPageCount() + 1 //initially 1, so adding one page needs to make it 2
	meta.SetPageCount(&pc)
	if err := pm.WritePage(0, &meta); err != nil {
		return 0, err
	}
	var blank Page
	y := PageID(pc) - 1
	if err := pm.WritePage(PageID(y), &blank); err != nil {
		return 0, err
	}
	return y, nil
}

func (pm *PageManager) FreePage(id PageID) error {
	if id == InvalidPageID {
		return ErrInvalidPageID
	}
	//if freeapge on same page is called twice it will set next to itself
	//so check that
	//create blank page
	//set type to free
	//set its next free to meta free list head
	//write page at id
	//set meta flh to id
	//write meta back to 0
	var meta Page
	if err := pm.ReadPage(0, &meta); err != nil {
		return err
	}
	var p Page 
	pm.ReadPage(id,&p)
	if p.GetPageType() == pageTypeFree{
		return ErrDoubleFree
	}
	var blank Page
	t := pageTypeFree
	blank.SetPageType(&t)
	blank.SetNextFreePageID(meta.GetFreeListHead())
	if err := pm.WritePage(id, &blank); err != nil {
		return err
	}
	x := freeListHead(id)
	meta.SetFreeListHead(&x)
	if err := pm.WritePage(0, &meta); err != nil {
		return err
	}
	return nil
}

