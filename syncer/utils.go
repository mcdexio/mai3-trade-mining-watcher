package syncer

func norm(ts int64) int64 {
	return ts - ts%60
}

func normN(ts int64, inv int64) int64 {
	return ts - ts%inv
}
