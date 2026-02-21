package kit

type ModalDims struct {
	ModalWidth  int
	ModalHeight int
	ListHeight  int
}

func ComputeModalDims(screenW, screenH, targetW, targetH, minW, minH, margin, listPad int) ModalDims {
	maxModalW := maxInt(1, screenW-margin)
	modalWidth := min(targetW, maxModalW)
	minModalW := min(minW, maxModalW)
	if modalWidth < minModalW {
		modalWidth = minModalW
	}

	maxModalH := maxInt(1, screenH-margin)
	modalHeight := min(targetH, maxModalH)
	minModalH := min(minH, maxModalH)
	if modalHeight < minModalH {
		modalHeight = minModalH
	}

	listHeight := modalHeight - listPad
	if listHeight < 4 {
		listHeight = 4
	}

	return ModalDims{
		ModalWidth:  modalWidth,
		ModalHeight: modalHeight,
		ListHeight:  listHeight,
	}
}
