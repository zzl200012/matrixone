package dataio

import (
	"fmt"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/common"

	"github.com/RoaringBitmap/roaring"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/container/batch"
)

var SegmentFileMockFactory = func(dir string, id uint64) SegmentFile {
	return mockSegment(dir, id)
}

type mockBlockFile struct {
	NoopBlockFile
	id         uint64
	rows       uint32
	segFile    SegmentFile
	data       batch.IBatch
	masks      map[uint16]*roaring.Bitmap
	vals       map[uint16]map[uint32]interface{}
	deletes    *roaring.Bitmap
	maxVisible uint64
}

type mockSegmentFile struct {
	NoopSegmentFile
	files  map[uint64]*mockBlockFile
	name   string
	sorted bool
}

func mockBlock(id uint64, bat batch.IBatch, segFile SegmentFile) *mockBlockFile {
	return &mockBlockFile{
		id:      id,
		segFile: segFile,
		data:    bat,
	}
}

func mockSegment(dir string, id uint64) *mockSegmentFile {
	name := fmt.Sprintf("%s-mock-%d", dir, id)
	return &mockSegmentFile{
		files: make(map[uint64]*mockBlockFile),
		name:  name,
	}
}

func (bf *mockBlockFile) Rows() uint32 { return bf.rows }

func (bf *mockBlockFile) GetSegmentFile() SegmentFile { return bf.segFile }

func (bf *mockBlockFile) WriteData(bat batch.IBatch, ts uint64, masks map[uint16]*roaring.Bitmap, vals map[uint16]map[uint32]interface{}, deletes *roaring.Bitmap) error {
	bf.data = bat
	bf.rows = uint32(bat.Length())
	bf.maxVisible = ts
	bf.masks = masks
	bf.vals = vals
	bf.deletes = deletes
	return nil
}

func (bf *mockBlockFile) LoadData() (bat batch.IBatch, err error) {
	bat = bf.data
	return
}

func (bf *mockBlockFile) GetMaxVisble() uint64 {
	return bf.maxVisible
}

func (sf *mockSegmentFile) IsSorted() bool { return sf.sorted }
func (sf *mockSegmentFile) GetBlockFile(id uint64) BlockFile {
	bf := sf.files[id]
	if bf == nil {
		bf = mockBlock(id, nil, sf)
		sf.files[id] = bf
	}
	return bf
}

func (sf *mockSegmentFile) Destory() error {
	for _, bf := range sf.files {
		if err := bf.Destory(); err != nil {
			return err
		}
	}
	return nil
}

type mockIndexFile struct {
	counter int
	data []byte
}

func MockIndexFile() *mockIndexFile {
	return &mockIndexFile{
		counter: 0,
		data:    make([]byte, 0),
	}
}

func (file *mockIndexFile) Append(data []byte) (startOffset uint32, err error) {
	startOffset = uint32(len(file.data))
	file.data = append(file.data, data...)
	return
}

func (file *mockIndexFile) Read(offset uint32, size uint32) (data []byte) {
	return file.data[offset:offset+size]
}

func (file *mockIndexFile) AllocIndexNodeId() common.ID {
	file.counter++
	return common.ID{
		TableID:   0,
		SegmentID: 0,
		BlockID:   0,
		PartID:    uint32(file.counter),
		Idx: uint16(file.counter),
		Iter:      0,
	}
}
