package errors

import "reflect"

// RecoverErrorPanic recovers an panic if needed. If panic recovered, it set the panic content
// to pErr and set pRet (return value) to default value. Useful when using assertions in types.
func RecoverErrorPanic(pErr *error, pRet interface{}) {
	if r := recover(); r != nil {
		// Catch parsing errors.
		panicErr, ok := r.(error)
		if !ok {
			// Reraise.
			panic(r)
		}
		// Set the error in panic to the content of pErr.
		*pErr = panicErr
		// Set the value inside pRet to zero value.
		ret := reflect.ValueOf(pRet).Elem()
		ret.Set(reflect.New(ret.Type()).Elem())
	}
}
