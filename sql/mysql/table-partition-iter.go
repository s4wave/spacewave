package mysql

import (
	"io"

	"github.com/dolthub/go-mysql-server/sql"
)

// TablePartitionIter is a table partition iterator.
type TablePartitionIter struct {
	// t is the table
	t *Table
	//  i is the partition index
	// -1 if closed
	i int
}

// NewTablePartitionIter constructs a new PartitionIter.
func NewTablePartitionIter(t *Table) *TablePartitionIter {
	return &TablePartitionIter{
		t: t,
	}
}

// Next iterates to the next partition.
func (i *TablePartitionIter) Next() (sql.Partition, error) {
	ix := i.i
	if ix < 0 {
		return nil, io.EOF
	}
	pts := i.t.root.GetTablePartitions()
	if ix >= len(pts) {
		return nil, io.EOF
	}
	i.i++
	return i.t.PartitionAtIndex(ix)
}

// Close closes the iterator.
func (i *TablePartitionIter) Close(sctx *sql.Context) error {
	i.i = -1
	return nil
}

// _ is a type assertion
var _ sql.PartitionIter = ((*TablePartitionIter)(nil))
