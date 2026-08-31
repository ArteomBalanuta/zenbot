package persistence

type Column struct {
	Name, Type string
	Nullable   bool
}
type Table struct {
	Name    string
	Columns []Column
}
type Schema struct{ Tables []Table }
