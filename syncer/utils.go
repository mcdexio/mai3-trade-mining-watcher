package syncer

import (
	"strconv"
	"strings"
)

func norm(ts int64) int64 {
	return ts - ts%60
}

func normN(ts int64, inv int64) int64 {
	return ts - ts%inv
}

func splitMarginAccountID(marginAccountID string) (poolAddr, userId string, perpetualIndex int, err error) {
	rest := strings.Split(marginAccountID, "-")
	perpetualIndex, err = strconv.Atoi(rest[1])
	if err != nil {
		return
	}
	poolAddr = rest[0]
	userId = rest[2]
	return
}
