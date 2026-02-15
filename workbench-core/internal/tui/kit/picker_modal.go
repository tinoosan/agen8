package kit

import "github.com/charmbracelet/lipgloss"

func DefaultPickerModalOpts(content string, screenW, screenH, modalW, modalH int) ModalOptions {
	return ModalOptions{
		Content:      content,
		ScreenWidth:  screenW,
		ScreenHeight: screenH,
		Width:        modalW,
		Height:       modalH,
		Padding:      [2]int{1, 2},
		BorderStyle:  lipgloss.RoundedBorder(),
		BorderColor:  lipgloss.Color("#6bbcff"),
		Foreground:   lipgloss.Color("#eaeaea"),
	}
}
