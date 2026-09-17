package index

import "encoding/json"

func resumeLaunchArgsJSON(args []string) string {
	if len(args) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return "[]"
	}
	return string(raw)
}
