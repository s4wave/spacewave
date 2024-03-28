// Package fetch is a js fetch wrapper that avoids importing net/http.
/*
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := fetch.Fetch("/some/api/call", &fetch.Opts{
		Body:   strings.NewReader(`{"one": "two"}`),
		Method: fetch.MethodPost,
		Signal: ctx,
	})
*/
package fetch
