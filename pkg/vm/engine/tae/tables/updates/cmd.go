package updates

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/iface/txnif"
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/txn/txnbase"
)

func init() {
	txnif.RegisterCmdFactory(txnbase.CmdDelete, func(int16) txnif.TxnCmd {
		return NewEmptyCmd(txnbase.CmdDelete)
	})
	txnif.RegisterCmdFactory(txnbase.CmdUpdate, func(int16) txnif.TxnCmd {
		return NewEmptyCmd(txnbase.CmdUpdate)
	})
	txnif.RegisterCmdFactory(txnbase.CmdAppend, func(int16) txnif.TxnCmd {
		return NewEmptyCmd(txnbase.CmdAppend)
	})
}

type UpdateCmd struct {
	*txnbase.BaseCustomizedCmd
	update  *ColumnNode
	delete  *DeleteNode
	append  *AppendNode
	cmdType int16
}

func NewEmptyCmd(cmdType int16) *UpdateCmd {
	cmd := NewUpdateCmd(0, nil)
	cmd.cmdType = cmdType
	if cmdType == txnbase.CmdUpdate {
		cmd.update = NewColumnNode(nil, nil, nil)
	} else if cmdType == txnbase.CmdDelete {
		cmd.delete = NewDeleteNode(nil)
	} else if cmdType == txnbase.CmdAppend {
		cmd.append = NewAppendNode(nil, 0, nil)
	}
	return cmd
}

func NewAppendCmd(id uint32, app *AppendNode) *UpdateCmd {
	impl := &UpdateCmd{
		append:  app,
		cmdType: txnbase.CmdAppend,
	}
	impl.BaseCustomizedCmd = txnbase.NewBaseCustomizedCmd(id, impl)
	return impl
}

func NewDeleteCmd(id uint32, del *DeleteNode) *UpdateCmd {
	impl := &UpdateCmd{
		delete:  del,
		cmdType: txnbase.CmdDelete,
	}
	impl.BaseCustomizedCmd = txnbase.NewBaseCustomizedCmd(id, impl)
	return impl
}

func NewUpdateCmd(id uint32, update *ColumnNode) *UpdateCmd {
	impl := &UpdateCmd{
		update:  update,
		cmdType: txnbase.CmdUpdate,
	}
	impl.BaseCustomizedCmd = txnbase.NewBaseCustomizedCmd(id, impl)
	return impl
}

// TODO
func (c *UpdateCmd) String() string {
	return ""
}

func (c *UpdateCmd) GetType() int16 { return c.cmdType }

func (c *UpdateCmd) WriteTo(w io.Writer) (err error) {
	if err = binary.Write(w, binary.BigEndian, c.GetType()); err != nil {
		return
	}
	if err = binary.Write(w, binary.BigEndian, c.ID); err != nil {
		return
	}
	if c.GetType() == txnbase.CmdUpdate {
		err = c.update.WriteTo(w)
	} else if c.GetType() == txnbase.CmdDelete {
		err = c.delete.WriteTo(w)
	} else {
		// TODO
	}
	return
}

func (c *UpdateCmd) ReadFrom(r io.Reader) (err error) {
	if err = binary.Read(r, binary.BigEndian, &c.ID); err != nil {
		return
	}
	if c.cmdType == txnbase.CmdUpdate {
		err = c.update.ReadFrom(r)
	} else if c.cmdType == txnbase.CmdDelete {
		err = c.delete.ReadFrom(r)
	} else {
		// TODO
	}
	return
}

func (c *UpdateCmd) Marshal() (buf []byte, err error) {
	var bbuf bytes.Buffer
	if err = c.WriteTo(&bbuf); err != nil {
		return
	}
	buf = bbuf.Bytes()
	return
}

func (c *UpdateCmd) Unmarshal(buf []byte) error {
	bbuf := bytes.NewBuffer(buf)
	return c.ReadFrom(bbuf)
}