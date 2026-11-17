//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// B2APIURL is where B2's native API authorizes an account.
const B2APIURL = "https://api.backblazeb2.com"

// b2MaxAnswer bounds the bytes read from one B2 answer.
const b2MaxAnswer = 4 << 20

// B2Account is the application key that reads a bucket's lifecycle rules
// through B2's native API. It needs the listBuckets capability. It prints
// without the key.
type B2Account struct {
	// APIURL is where to authorize; empty uses B2APIURL.
	APIURL string
	// KeyID is the application key id, the S3 access key id.
	KeyID string
	// Key is the application key, the S3 secret access key.
	Key string
	// HTTPClient sends the requests; nil uses one with a 30 second timeout.
	HTTPClient *http.Client
}

// String names the account by its key id alone.
func (a B2Account) String() string {
	return "B2Account{KeyID: " + a.KeyID + "}"
}

// GoString names the account by its key id alone.
func (a B2Account) GoString() string {
	return a.String()
}

// ReadB2LifecycleRules authorizes account with b2_authorize_account and reads
// the lifecycle rules of bucket with b2_list_buckets. It refuses an answer
// without exactly that bucket or whose lifecycleRules is not a list of rules
// that each name a fileNamePrefix.
func ReadB2LifecycleRules(ctx context.Context, account B2Account, bucket string) ([]LifecycleRule, error) {
	// Fill in the defaults.
	base := account.APIURL
	if base == "" {
		base = B2APIURL
	}
	httpClient := account.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	// Authorize the key.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+"/b2api/v4/b2_authorize_account", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(account.KeyID, account.Key)
	var parser fastjson.Parser
	auth, err := b2Call(httpClient, req, "b2_authorize_account", &parser)
	if err != nil {
		return nil, err
	}

	// Keep the account, the token and the API's URL.
	accountID := string(auth.GetStringBytes("accountId"))
	token := string(auth.GetStringBytes("authorizationToken"))
	apiURL := string(auth.GetStringBytes("apiInfo", "storageApi", "apiUrl"))
	if accountID == "" || token == "" || apiURL == "" {
		return nil, errors.New("b2_authorize_account: the answer lacks the account, the token or the apiUrl")
	}

	// Ask for the bucket by name, which a key restricted to it requires.
	var arena fastjson.Arena
	query := arena.NewObject()
	query.Set("accountId", arena.NewString(accountID))
	query.Set("bucketName", arena.NewString(bucket))
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(apiURL, "/")+"/b2api/v4/b2_list_buckets", bytes.NewReader(query.MarshalTo(nil)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	// Take the one bucket's rules.
	list, err := b2Call(httpClient, req, "b2_list_buckets", &parser)
	if err != nil {
		return nil, err
	}
	buckets := list.GetArray("buckets")
	if len(buckets) != 1 || string(buckets[0].GetStringBytes("bucketName")) != bucket {
		return nil, errors.Errorf("b2_list_buckets: the answer lists %d buckets, not %s alone", len(buckets), bucket)
	}
	return parseLifecycleRules(buckets[0].Get("lifecycleRules"))
}

// b2Call sends req and parses its JSON answer with parser. A failed answer
// becomes an error naming the call, the status and B2's error code, and never
// the request, which carries the key or the token.
func b2Call(httpClient *http.Client, req *http.Request, call string, parser *fastjson.Parser) (*fastjson.Value, error) {
	// Send the request and read a bounded answer.
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.Errorf("%s: the request failed", call)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, b2MaxAnswer))
	if err != nil {
		return nil, errors.Wrap(err, call+": read the answer")
	}

	// Parse it, naming only the status and code of a failure.
	value, perr := parser.ParseBytes(data)
	if resp.StatusCode != http.StatusOK {
		code := ""
		if perr == nil {
			code = string(value.GetStringBytes("code"))
		}
		return nil, errors.Errorf("%s: status %d, code %q", call, resp.StatusCode, code)
	}
	if perr != nil {
		return nil, errors.Wrap(perr, call+": parse the answer")
	}
	return value, nil
}
