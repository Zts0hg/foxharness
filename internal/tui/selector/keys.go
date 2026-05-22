package selector

type keyMap struct {
	up      []string
	down    []string
	selectK []string
	cancel  []string
}

var keys = keyMap{
	up:      []string{"up", "k"},
	down:    []string{"down", "j"},
	selectK: []string{"enter"},
	cancel:  []string{"esc", "q"},
}

func keyMatches(key string, values []string) bool {
	for _, value := range values {
		if key == value {
			return true
		}
	}
	return false
}
