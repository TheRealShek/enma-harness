package core

func Percent(part, total int) int {
	if total == 0 {
		return 0
	}

	return part / total * 100
}
