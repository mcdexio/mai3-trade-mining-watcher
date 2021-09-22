package http

type IHttpClient interface {
	Request(method, url string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
	Get(url string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
	Post(url string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
	Delete(url string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
	Put(url string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte)
}
