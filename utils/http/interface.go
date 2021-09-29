package http

type IHttpClient interface {
	Request(method string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
	Get(params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
	Post(params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
	Delete(params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
	Put(params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
}
