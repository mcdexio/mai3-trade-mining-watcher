package syncer

import (
	"fmt"
	"strconv"
	"strings"
)

func norm(ts int64) int64 {
	return ts - ts%60
}

func normN(ts int64, inv int64) int64 {
	return ts - ts%inv
}

func splitMarginAccountID(marginAccountID string) (poolAddr, userId string, perpIndex int, err error) {
	rest := strings.Split(marginAccountID, "-")
	perpIndex, err = strconv.Atoi(rest[1])
	if err != nil {
		err = fmt.Errorf("fail to spliMarginAccountID from id=%s err=%s", marginAccountID, err)
		return
	}
	poolAddr = rest[0]
	userId = rest[2]
	return
}
