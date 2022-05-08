package txnentries

import (
	"sync"

	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/catalog"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/common"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/container/compute"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/iface/handle"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/iface/txnif"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/tables/updates"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/tasks"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/wal"
)

type compactBlockEntry struct {
	sync.RWMutex
	txn       txnif.AsyncTxn
	from      handle.Block
	to        handle.Block
	scheduler tasks.TaskScheduler
}

func NewCompactBlockEntry(txn txnif.AsyncTxn, from, to handle.Block, scheduler tasks.TaskScheduler) *compactBlockEntry {
	return &compactBlockEntry{
		txn:       txn,
		from:      from,
		to:        to,
		scheduler: scheduler,
	}
}

func (entry *compactBlockEntry) PrepareRollback() (err error) {
	// TODO: remove block file? (should be scheduled and executed async)
	return
}
func (entry *compactBlockEntry) ApplyRollback() (err error) { return }
func (entry *compactBlockEntry) ApplyCommit(index *wal.Index) (err error) {
	entry.scheduler.Checkpoint([]*wal.Index{index})
	entry.PostCommit()
	return
}
func (entry *compactBlockEntry) PostCommit() {
	meta := entry.from.GetMeta().(*catalog.BlockEntry)
	entry.scheduler.ScheduleScopedFn(nil, tasks.CheckpointTask, meta.AsCommonID(), meta.GetBlockData().CheckpointWALClosure(entry.txn.GetCommitTS()))
}
func (entry *compactBlockEntry) MakeCommand(csn uint32) (cmd txnif.TxnCmd, err error) {
	cmd = newCompactBlockCmd((*common.ID)(entry.from.Fingerprint()), (*common.ID)(entry.to.Fingerprint()))
	return
}

func (entry *compactBlockEntry) PrepareCommit() (err error) {
	dataBlock := entry.from.GetMeta().(*catalog.BlockEntry).GetBlockData()
	v := dataBlock.CollectChangesInRange(entry.txn.GetStartTS(), entry.txn.GetCommitTS())
	view := v.(*updates.BlockView)
	if view == nil {
		return
	}
	deletes := view.DeleteMask
	for colIdx, mask := range view.UpdateMasks {
		vals := view.UpdateVals[colIdx]
		view.UpdateMasks[colIdx], view.UpdateVals[colIdx], view.DeleteMask = compute.ShuffleByDeletes(mask, vals, deletes)
		for row, v := range view.UpdateVals[colIdx] {
			if err = entry.to.Update(row, colIdx, v); err != nil {
				return
			}
		}
	}
	if len(view.UpdateMasks) == 0 {
		_, _, view.DeleteMask = compute.ShuffleByDeletes(nil, nil, view.DeleteMask)
	}
	if view.DeleteMask != nil {
		it := view.DeleteMask.Iterator()
		for it.HasNext() {
			row := it.Next()
			if err = entry.to.RangeDelete(row, row); err != nil {
				return
			}
		}
	}
	return
}
