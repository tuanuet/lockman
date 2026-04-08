package tui

type ScreenRefreshMsg struct{}

type ScreenSwitchMsg int

type ErrToastMsg string

type ClearToastMsg struct{}

func ScreenSwitchTo(idx int) ScreenSwitchMsg {
	return ScreenSwitchMsg(idx)
}

func NextScreen(current, total int) int {
	return (current + 1) % total
}

func PrevScreen(current, total int) int {
	idx := current - 1
	if idx < 0 {
		idx = total - 1
	}
	return idx
}
