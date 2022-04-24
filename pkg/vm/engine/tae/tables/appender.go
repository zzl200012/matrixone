package tables

import (
	gbat "github.com/matrixorigin/matrixone/pkg/container/batch"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/buffer/base"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/common"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/iface/txnif"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/index/access/acif"
)

type blockAppender struct {
	node          *appendableNode
	handle        base.INodeHandle
	indexAppender acif.IAppendableBlockIndexHolder
	placeholder   uint32
	rows          uint32
}

func newAppender(node *appendableNode, idxApd acif.IAppendableBlockIndexHolder) *blockAppender {
	appender := new(blockAppender)
	appender.node = node
	appender.handle = node.mgr.Pin(node)
	appender.indexAppender = idxApd
	appender.rows = node.Rows(nil, true)
	return appender
}

func (appender *blockAppender) Close() error {
	if appender.handle != nil {
		appender.handle.Close()
		appender.handle = nil
	}
	return nil
}

func (appender *blockAppender) GetID() *common.ID {
	return appender.node.block.meta.AsCommonID()
}

func (appender *blockAppender) IsAppendable() bool {
	return appender.rows+appender.placeholder < appender.node.block.meta.GetSchema().BlockMaxRows
}

func (appender *blockAppender) PrepareAppend(rows uint32) (n uint32, err error) {
	left := appender.node.block.meta.GetSchema().BlockMaxRows - appender.rows - appender.placeholder
	if left == 0 {
		return
	}
	if rows > left {
		n = left
	} else {
		n = rows
	}
	appender.placeholder += n
	return
}

func (appender *blockAppender) ApplyAppend(bat *gbat.Batch, offset, length uint32, txn txnif.AsyncTxn) (node txnif.AppendNode, from uint32, err error) {
	writeLock := appender.node.block.controller.GetExclusiveLock()
	defer writeLock.Unlock()
	err = appender.node.Expand(0, func() error {
		var err error
		from, err = appender.node.ApplyAppend(bat, offset, length, txn)
		return err
	})

	pks := bat.Vecs[appender.node.block.meta.GetSchema().PrimaryKey]
	// logutil.Infof("Append into %d: %s", appender.node.meta.GetID(), pks.String())
	err = appender.indexAppender.BatchInsert(pks, offset, int(length), from, false)
	if err != nil {
		panic(err)
	}
	node = appender.node.block.controller.AddAppendNodeLocked(txn, appender.node.rows)
	// appender.node.block.controller.SetMaxVisible(txn.GetCommitTS())

	return
}