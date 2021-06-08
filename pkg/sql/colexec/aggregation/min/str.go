package min

import (
	"bytes"
	"matrixone/pkg/container/types"
	"matrixone/pkg/container/vector"
	"matrixone/pkg/sql/colexec/aggregation"
	"matrixone/pkg/vectorize/min"
	"matrixone/pkg/vm/mempool"
	"matrixone/pkg/vm/process"
)

func NewStr(typ types.Type) *strMin {
	return &strMin{typ: typ, v: make([]byte, 0, 8)}
}

func (a *strMin) Reset() {
	a.cnt = 0
	a.v = a.v[:0]
}

func (a *strMin) Type() types.Type {
	return a.typ
}

func (a *strMin) Dup() aggregation.Aggregation {
	return &strMin{typ: a.typ, v: make([]byte, 0, 8)}
}

func (a *strMin) Fill(sels []int64, vec *vector.Vector) error {
	if n := len(sels); n > 0 {
		v := min.StrMinSels(vec.Col.(*types.Bytes), sels)
		if a.cnt == 0 || bytes.Compare(v, a.v) < 0 {
			a.v = append(a.v[:0], v...)
		}
		a.cnt += int64(n - vec.Nsp.FilterCount(sels))
	} else {
		v := min.StrMin(vec.Col.(*types.Bytes))
		if a.cnt == 0 || bytes.Compare(v, a.v) < 0 {
			a.v = append(a.v[:0], v...)
		}
		a.cnt += int64(vec.Length() - vec.Nsp.Length())
	}
	return nil
}

func (a *strMin) Eval() interface{} {
	if a.cnt == 0 {
		return nil
	}
	return a.v
}

func (a *strMin) EvalCopy(proc *process.Process) (*vector.Vector, error) {
	length := len(a.v)
	if length == 0 {
		length++
	}
	data, err := proc.Alloc(int64(length))
	if err != nil {
		return nil, err
	}
	vec := vector.New(a.typ)
	col := vec.Col.(*types.Bytes)
	col.Data = data[mempool.CountSize:mempool.CountSize]
	col.Offsets = append(col.Offsets, 0)
	if a.cnt == 0 {
		vec.Nsp.Add(0)
	} else {
		col.Data = append(col.Data, a.v...)
		col.Lengths = append(col.Lengths, uint32(len(a.v)))
	}
	vec.Col = col
	vec.Data = data
	return vec, nil
}