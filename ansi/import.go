package ansi

// Import adds the provided table to the list of known sequences.
// Duplicated entries are ignored and returned as the list of Names.
func Import(table map[Name]*Sequence) []Name {
	var dups []Name
	for name, seq := range table {
		if Table[name] != nil {
			dups = append(dups, name)
			continue
		}
		Table[name] = seq
	}
	return dups
}
