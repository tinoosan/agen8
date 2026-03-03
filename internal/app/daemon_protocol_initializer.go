package app

func shouldEnableProtocolStdio(explicit bool, inTTY, outTTY bool) bool {
	if explicit {
		return true
	}
	return !inTTY && !outTTY
}
