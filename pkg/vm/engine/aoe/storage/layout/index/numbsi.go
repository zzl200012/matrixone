// Copyright 2021 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package index

import (
	"bytes"
	"github.com/RoaringBitmap/roaring"
	"github.com/matrixorigin/matrixone/pkg/container/types"
	"github.com/matrixorigin/matrixone/pkg/encoding"
	buf "github.com/matrixorigin/matrixone/pkg/vm/engine/aoe/storage/buffer"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/aoe/storage/common"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/aoe/storage/layout/base"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/index/bsi"
	"io"
	// log "github.com/sirupsen/logrus"
)

func NumericBsiIndexConstructor(vf common.IVFile, useCompress bool, freeFunc buf.MemoryFreeFunc) buf.IMemoryNode {
	return NewNumericBsiEmptyNode(vf, useCompress, freeFunc)
}

type NumericBsiIndex struct {
	bsi.NumericBSI
	T           types.Type
	Col         int16
	File        common.IVFile
	UseCompress bool
	FreeFunc    buf.MemoryFreeFunc
	Offset uint64
}

func NewNumericBsiIndex(t types.Type, bitSize int, colIdx int16, startPos uint64) *NumericBsiIndex {
	bsiIdx := getNumericBsi(t, bitSize)
	return &NumericBsiIndex{
		T:          t,
		Col:        colIdx,
		NumericBSI: *bsiIdx,
		Offset: startPos,
	}
}

func getNumericBsi(t types.Type, bitSize int) *bsi.NumericBSI {
	var bsiIdx bsi.BitSlicedIndex
	switch t.Oid {
	case types.T_int8, types.T_int16, types.T_int32, types.T_int64, types.T_date, types.T_datetime:
		bsiIdx = bsi.NewNumericBSI(bitSize, bsi.SignedInt)
	case types.T_uint8, types.T_uint16, types.T_uint32, types.T_uint64:
		bsiIdx = bsi.NewNumericBSI(bitSize, bsi.UnsignedInt)
	case types.T_float32, types.T_float64:
		bsiIdx = bsi.NewNumericBSI(bitSize, bsi.Float)
	default:
		panic("not supported")
	}
	return bsiIdx.(*bsi.NumericBSI)
}

func NewNumericBsiEmptyNode(vf common.IVFile, useCompress bool, freeFunc buf.MemoryFreeFunc) buf.IMemoryNode {
	return &NumericBsiIndex{
		File:        vf,
		UseCompress: useCompress,
		FreeFunc:    freeFunc,
	}
}

func (i *NumericBsiIndex) GetCol() int16 {
	return i.Col
}

func (i *NumericBsiIndex) Eval(ctx *FilterCtx) error {
	if ctx.BMRes.IsEmpty() {
		return nil
	}
	var err error
	if v, ok := ctx.Val.(types.Date); ok {
		ctx.Val = int32(v)
		defer func() {
			ctx.Val = v
		}()
	}
	if v, ok := ctx.ValMin.(types.Date); ok {
		ctx.ValMin = int32(v)
		defer func() {
			ctx.ValMin = v
		}()
	}
	if v, ok := ctx.ValMax.(types.Date); ok {
		ctx.ValMax = int32(v)
		defer func() {
			ctx.ValMax = v
		}()
	}
	if v, ok := ctx.Val.(types.Datetime); ok {
		ctx.Val = int64(v)
		defer func() {
			ctx.Val = v
		}()
	}
	if v, ok := ctx.ValMin.(types.Datetime); ok {
		ctx.ValMin = int64(v)
		defer func() {
			ctx.ValMin = v
		}()
	}
	if v, ok := ctx.ValMax.(types.Datetime); ok {
		ctx.ValMax = int64(v)
		defer func() {
			ctx.ValMax = v
		}()
	}
	switch ctx.Op {
	case OpEq:
		ctx.BMRes, err = i.Eq(ctx.Val, ctx.BMRes)
	case OpNe:
		ctx.BMRes, err = i.Ne(ctx.Val, ctx.BMRes)
	case OpGe:
		ctx.BMRes, err = i.Ge(ctx.Val, ctx.BMRes)
	case OpGt:
		ctx.BMRes, err = i.Gt(ctx.Val, ctx.BMRes)
	case OpLe:
		ctx.BMRes, err = i.Le(ctx.Val, ctx.BMRes)
	case OpLt:
		ctx.BMRes, err = i.Lt(ctx.Val, ctx.BMRes)
	case OpIn:
		bm := ctx.BMRes.Clone()
		ctx.BMRes, err = i.Ge(ctx.ValMin, ctx.BMRes)
		if err != nil {
			return err
		}
		bm, err = i.Le(ctx.ValMax, bm)
		if err != nil {
			return err
		}
		ctx.BMRes.And(bm)
	case OpOut:
		bm := ctx.BMRes.Clone()
		ctx.BMRes, err = i.Gt(ctx.ValMax, ctx.BMRes)
		if err != nil {
			return err
		}
		bm, err = i.Lt(ctx.ValMin, bm)
		if err != nil {
			return err
		}
		ctx.BMRes.Or(bm)
	}
	return err
}

func (i *NumericBsiIndex) FreeMemory() {
	if i.FreeFunc != nil {
		i.FreeFunc(i)
	}
}

func (i *NumericBsiIndex) IndexFile() common.IVFile {
	return i.File
}

func (i *NumericBsiIndex) Type() base.IndexType {
	return base.NumBsi
}

func (i *NumericBsiIndex) GetMemorySize() uint64 {
	if i.UseCompress {
		return uint64(i.File.Stat().Size())
	} else {
		return uint64(i.File.Stat().OriginSize())
	}
}

func (i *NumericBsiIndex) GetMemoryCapacity() uint64 {
	if i.UseCompress {
		return uint64(i.File.Stat().Size())
	} else {
		return uint64(i.File.Stat().OriginSize())
	}
}

func (i *NumericBsiIndex) Reset() {
}

func (i *NumericBsiIndex) ReadFrom(r io.Reader) (n int64, err error) {
	buf := make([]byte, i.GetMemoryCapacity())
	nr, err := r.Read(buf)
	if err != nil {
		return int64(nr), err
	}
	err = i.Unmarshal(buf)
	return int64(nr), err
}

func (i *NumericBsiIndex) WriteTo(w io.Writer) (n int64, err error) {
	buf, err := i.Marshal()
	if err != nil {
		return n, err
	}

	nw, err := w.Write(buf)
	return int64(nw), err
}

func (i *NumericBsiIndex) Unmarshal(data []byte) error {
	buf := data
	i.Col = encoding.DecodeInt16(buf[:2])
	buf = buf[2:]
	i.T = encoding.DecodeType(buf[:encoding.TypeSize])
	buf = buf[encoding.TypeSize:]
	i.Offset = encoding.DecodeUint64(buf[:8])
	buf = buf[8:]
	return i.NumericBSI.Unmarshal(buf)
}

func (i *NumericBsiIndex) Marshal() ([]byte, error) {
	var bw bytes.Buffer
	bw.Write(encoding.EncodeInt16(i.Col))
	bw.Write(encoding.EncodeType(i.T))
	bw.Write(encoding.EncodeUint64(i.Offset))
	indexBuf, err := i.NumericBSI.Marshal()
	if err != nil {
		return nil, err
	}
	bw.Write(indexBuf)
	return bw.Bytes(), nil
}

func (i *NumericBsiIndex) Get(k uint64) (interface{}, bool) {
	k = k - i.Offset
	return i.NumericBSI.Get(k)
}

func (i *NumericBsiIndex) Set(k uint64, e interface{}) error {
	k = k - i.Offset
	return i.NumericBSI.Set(k, e)
}

func (i *NumericBsiIndex) Eq(e interface{}, filter *roaring.Bitmap) (*roaring.Bitmap, error) {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	res, err := i.NumericBSI.Eq(e, filter)
	if err != nil {
		return nil, err
	}
	if res != nil {
		arr := res.ToArray()
		out := roaring.NewBitmap()
		for _, n := range arr {
			out.Add(n + uint32(i.Offset))
		}
		res = out
	}
	return res, nil
}

func (i *NumericBsiIndex) Ne(e interface{}, filter *roaring.Bitmap) (*roaring.Bitmap, error) {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	res, err := i.NumericBSI.Ne(e, filter)
	if err != nil {
		return nil, err
	}
	if res != nil {
		arr := res.ToArray()
		out := roaring.NewBitmap()
		for _, n := range arr {
			out.Add(n + uint32(i.Offset))
		}
		res = out
	}
	return res, nil
}

func (i *NumericBsiIndex) Lt(e interface{}, filter *roaring.Bitmap) (*roaring.Bitmap, error) {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	res, err := i.NumericBSI.Lt(e, filter)
	if err != nil {
		return nil, err
	}
	if res != nil {
		arr := res.ToArray()
		out := roaring.NewBitmap()
		for _, n := range arr {
			out.Add(n + uint32(i.Offset))
		}
		res = out
	}
	return res, nil
}

func (i *NumericBsiIndex) Le(e interface{}, filter *roaring.Bitmap) (*roaring.Bitmap, error) {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	res, err := i.NumericBSI.Le(e, filter)
	if err != nil {
		return nil, err
	}
	if res != nil {
		arr := res.ToArray()
		out := roaring.NewBitmap()
		for _, n := range arr {
			out.Add(n + uint32(i.Offset))
		}
		res = out
	}
	return res, nil
}

func (i *NumericBsiIndex) Gt(e interface{}, filter *roaring.Bitmap) (*roaring.Bitmap, error) {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	res, err := i.NumericBSI.Gt(e, filter)
	if err != nil {
		return nil, err
	}
	if res != nil {
		arr := res.ToArray()
		out := roaring.NewBitmap()
		for _, n := range arr {
			out.Add(n + uint32(i.Offset))
		}
		res = out
	}
	return res, nil
}

func (i *NumericBsiIndex) Ge(e interface{}, filter *roaring.Bitmap) (*roaring.Bitmap, error) {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	res, err := i.NumericBSI.Ge(e, filter)
	if err != nil {
		return nil, err
	}
	if res != nil {
		arr := res.ToArray()
		out := roaring.NewBitmap()
		for _, n := range arr {
			out.Add(n + uint32(i.Offset))
		}
		res = out
	}
	return res, nil
}

func (i *NumericBsiIndex) NotNull(filter *roaring.Bitmap) *roaring.Bitmap {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	res := i.NumericBSI.NotNull(filter)
	if res != nil {
		arr := res.ToArray()
		out := roaring.NewBitmap()
		for _, n := range arr {
			out.Add(n + uint32(i.Offset))
		}
		res = out
	}
	return res
}

func (i *NumericBsiIndex) Count(filter *roaring.Bitmap) uint64 {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	return i.NumericBSI.Count(filter)
}

func (i *NumericBsiIndex) NullCount(filter *roaring.Bitmap) uint64 {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	return i.NumericBSI.NullCount(filter)
}

func (i *NumericBsiIndex) Min(filter *roaring.Bitmap) (interface{}, uint64) {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	return i.NumericBSI.Min(filter)
}

func (i *NumericBsiIndex) Max(filter *roaring.Bitmap) (interface{}, uint64) {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	return i.NumericBSI.Max(filter)
}

func (i *NumericBsiIndex) Sum(filter *roaring.Bitmap) (interface{}, uint64) {
	if filter != nil {
		arr := filter.ToArray()
		in := roaring.NewBitmap()
		for _, n := range arr {
			if uint64(n) < i.Offset {
				continue
			}
			in.Add(n - uint32(i.Offset))
		}
		filter = in
	}
	return i.NumericBSI.Sum(filter)
}
