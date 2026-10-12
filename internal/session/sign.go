package session

const MemoryWriteWord = "i-write-memory"

// Signable is a word `lgrass sign` accepts, with the pledge printed back once it is signed.
type Signable struct {
	ID     string
	Pledge string
}

// SignSource lists the signables one origin contributes for a project.
type SignSource func(projectPath string) ([]Signable, error)

var signSources = []SignSource{builtinSignables}

// ResolveSignable returns the first signable with this id across the sources, and false when none registers it.
func ResolveSignable(projectPath, id string) (Signable, bool, error) {
	for _, source := range signSources {
		list, err := source(projectPath)
		if err != nil {
			return Signable{}, false, err
		}
		for _, s := range list {
			if s.ID == id {
				return s, true, nil
			}
		}
	}
	return Signable{}, false, nil
}

func builtinSignables(string) ([]Signable, error) {
	return []Signable{
		{
			ID:     MemoryWriteWord,
			Pledge: "Memory holds one-line pointers only. I will not write a standing rule, a feedback entry or knowledge into it. Those go in biblio/laws/ or biblio/books/.",
		},
	}, nil
}
