package jobs

import (
	"github.com/matrixorigin/matrixone/pkg/container/batch"
	"github.com/matrixorigin/matrixone/pkg/logutil"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/aoe/mergesort"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/catalog"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/common"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/container/compute"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/iface/handle"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/iface/txnif"
	idxCommon "github.com/matrixorigin/matrixone/pkg/vm/engine/tae/index/common"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/tables/txnentries"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/tasks"
)

var CompactBlockTaskFactory = func(meta *catalog.BlockEntry, ioScheduler tasks.Scheduler) tasks.TxnTaskFactory {
	return func(ctx *tasks.Context, txn txnif.AsyncTxn) (tasks.Task, error) {
		return NewCompactBlockTask(ctx, txn, meta, ioScheduler)
	}
}

type compactBlockTask struct {
	*tasks.BaseTask
	txn         txnif.AsyncTxn
	compacted   handle.Block
	created     handle.Block
	meta        *catalog.BlockEntry
	ioScheduler tasks.Scheduler
}

func NewCompactBlockTask(ctx *tasks.Context, txn txnif.AsyncTxn, meta *catalog.BlockEntry, ioScheduler tasks.Scheduler) (task *compactBlockTask, err error) {
	task = &compactBlockTask{
		txn:         txn,
		meta:        meta,
		ioScheduler: ioScheduler,
	}
	dbName := meta.GetSegment().GetTable().GetDB().GetName()
	database, err := txn.GetDatabase(dbName)
	if err != nil {
		return
	}
	relName := meta.GetSchema().Name
	rel, err := database.GetRelationByName(relName)
	if err != nil {
		return
	}
	seg, err := rel.GetSegment(meta.GetSegment().GetID())
	if err != nil {
		return
	}
	task.compacted, err = seg.GetBlock(meta.GetID())
	if err != nil {
		return
	}
	task.BaseTask = tasks.NewBaseTask(task, tasks.CompactBlockTask, ctx)
	return
}

func (task *compactBlockTask) PrepareData() (bat *batch.Batch, err error) {
	attrs := task.meta.GetSchema().Attrs()
	bat = batch.New(true, attrs)

	for i := range task.meta.GetSchema().ColDefs {
		vec, mask, err := task.compacted.GetColumnDataById(i, nil, nil)
		if err != nil {
			return bat, err
		}
		vec = compute.ApplyDeleteToVector(vec, mask)
		bat.Vecs[i] = vec
	}
	if err = mergesort.SortBlockColumns(bat.Vecs, int(task.meta.GetSchema().PrimaryKey)); err != nil {
		return
	}
	return
}

func (task *compactBlockTask) GetNewBlock() handle.Block { return task.created }

func (task *compactBlockTask) Execute() (err error) {
	data, err := task.PrepareData()
	if err != nil {
		return
	}
	seg := task.compacted.GetSegment()
	// rel := seg.GetRelation()
	newBlk, err := seg.CreateNonAppendableBlock()
	if err != nil {
		return err
	}
	if err = seg.SoftDeleteBlock(task.compacted.Fingerprint().BlockID); err != nil {
		return err
	}
	newMeta := newBlk.GetMeta().(*catalog.BlockEntry)
	newBlkData := newMeta.GetBlockData()
	blockFile := newBlkData.GetBlockFile()

	ctx := tasks.Context{Waitable: true}
	ioTask := NewFlushBlkTask(&ctx, blockFile, task.txn.GetStartTS(), newMeta, data)
	if task.ioScheduler != nil {
		if err = task.ioScheduler.Schedule(ioTask); err != nil {
			return
		}
		if err = ioTask.WaitDone(); err != nil {
			return
		}
	} else {
		if err = ioTask.OnExec(); err != nil {
			return
		}
	}

	if err = newBlkData.RefreshIndex(); err != nil {
		return err
	}
	task.created = newBlk
	txnEntry := txnentries.NewCompactBlockEntry(task.txn, task.compacted, task.created)
	if err = task.txn.LogTxnEntry(task.meta.GetSegment().GetTable().GetID(), txnEntry, []*common.ID{task.compacted.Fingerprint()}); err != nil {
		return
	}
	logutil.Debug(idxCommon.MockIndexBufferManager.String())
	return
}