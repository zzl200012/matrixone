package tasks

import (
	"sync"

	"github.com/matrixorigin/matrixone/pkg/logutil"
	iops "github.com/matrixorigin/matrixone/pkg/vm/engine/tae/ops/base"
	ops "github.com/matrixorigin/matrixone/pkg/vm/engine/tae/worker"
	"github.com/panjf2000/ants/v2"
)

var (
	poolHandlerName = "PoolHandler"
)

type poolHandler struct {
	BaseTaskHandler
	opExec ops.OpExecFunc
	pool   *ants.Pool
	wg     *sync.WaitGroup
}

func NewPoolHandler(num int) *poolHandler {
	pool, err := ants.NewPool(num)
	if err != nil {
		panic(err)
	}
	h := &poolHandler{
		BaseTaskHandler: *NewBaseEventHandler(poolHandlerName),
		pool:            pool,
		wg:              &sync.WaitGroup{},
	}
	h.opExec = h.ExecFunc
	h.ExecFunc = h.doHandle
	return h
}

func (h *poolHandler) Execute(task Task) {
	h.opExec(task)
}

func (h *poolHandler) doHandle(op iops.IOp) {
	closure := func(o iops.IOp, wg *sync.WaitGroup) func() {
		return func() {
			h.opExec(o)
			wg.Done()
		}
	}
	h.wg.Add(1)
	err := h.pool.Submit(closure(op, h.wg))
	if err != nil {
		logutil.Warnf("%v", err)
	}
}

func (h *poolHandler) Close() error {
	h.BaseTaskHandler.Close()
	h.wg.Wait()
	return nil
}