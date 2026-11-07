package cdn_grant

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// B2APIURL is the Backblaze B2 native API origin accounts authorize at.
const B2APIURL = "https://api.backblazeb2.com"

// b2MaxTTL is the longest download authorization B2 issues.
const b2MaxTTL = 7 * 24 * time.Hour

// b2AccountTTL is how long an account authorization is reused. B2 issues
// them for 24 hours.
const b2AccountTTL = 23 * time.Hour

// B2Config configures a B2 download authorizer.
type B2Config struct {
	// KeyID and ApplicationKey are a B2 application key that may share files
	// of the bucket. They stay on the server.
	KeyID, ApplicationKey string
	// BucketID and BucketName name the private bucket.
	BucketID, BucketName string
	// APIURL overrides B2APIURL.
	APIURL string
	// HttpClient overrides http.DefaultClient.
	HttpClient *http.Client
}

// B2 mints B2 native download authorizations: a token that reads the
// bucket's files whose names start with a prefix, sent as the Authorization
// header of a download by name.
type B2 struct {
	cfg B2Config
	cli *http.Client

	// mtx guards the account authorization.
	mtx sync.Mutex
	// account is the current account authorization, or nil.
	account *b2Account
}

// b2Account is an account authorization.
type b2Account struct {
	token, apiURL, downloadURL string
	expires                    time.Time
}

// NewB2 returns a B2 download authorizer.
func NewB2(cfg B2Config) (*B2, error) {
	// Require the key and bucket, and default the API origin and client.
	if cfg.KeyID == "" || cfg.ApplicationKey == "" || cfg.BucketID == "" || cfg.BucketName == "" {
		return nil, errors.New("B2 key ID, application key, bucket ID and bucket name are required")
	}
	if cfg.APIURL == "" {
		cfg.APIURL = B2APIURL
	}
	cli := cfg.HttpClient
	if cli == nil {
		cli = http.DefaultClient
	}
	return &B2{cfg: cfg, cli: cli}, nil
}

// AuthorizeDownloads mints a download authorization for the files under
// prefix, good for ttl up to B2's limit of a week.
func (b *B2) AuthorizeDownloads(ctx context.Context, prefix string, ttl time.Duration) (*DownloadAuthorization, error) {
	// Bound the life to what B2 issues, in whole seconds.
	ttl = min(max(ttl, time.Second), b2MaxTTL)
	start := time.Now()
	var arena fastjson.Arena
	req := arena.NewObject()
	req.Set("bucketId", arena.NewString(b.cfg.BucketID))
	req.Set("fileNamePrefix", arena.NewString(prefix))
	req.Set("validDurationInSeconds", arena.NewNumberString(strconv.FormatInt(int64(ttl/time.Second), 10)))
	body := req.MarshalTo(nil)

	// Ask with the account authorization, renewing it once if B2 refuses it.
	acct, err := b.authorize(ctx, false)
	if err != nil {
		return nil, err
	}
	status, resp, err := b.call(ctx, acct.apiURL+"/b2api/v3/b2_get_download_authorization", acct.token, body)
	if status == http.StatusUnauthorized {
		if acct, err = b.authorize(ctx, true); err != nil {
			return nil, err
		}
		_, resp, err = b.call(ctx, acct.apiURL+"/b2api/v3/b2_get_download_authorization", acct.token, body)
	}
	if err != nil {
		return nil, errors.Wrap(err, "b2_get_download_authorization")
	}

	// Read the token scoped to the prefix.
	token := string(resp.GetStringBytes("authorizationToken"))
	if token == "" {
		return nil, errors.New("b2_get_download_authorization returned no token")
	}
	return &DownloadAuthorization{
		BaseURL:       strings.TrimSuffix(acct.downloadURL, "/") + "/file/" + url.PathEscape(b.cfg.BucketName),
		Authorization: token,
		Expires:       start.Add(ttl),
	}, nil
}

// authorize returns the account authorization, asking B2 for a new one when
// there is none, it is old, or renew is set.
func (b *B2) authorize(ctx context.Context, renew bool) (*b2Account, error) {
	// Reuse a current account authorization.
	b.mtx.Lock()
	defer b.mtx.Unlock()
	now := time.Now()
	if a := b.account; a != nil && !renew && now.Before(a.expires) {
		return a, nil
	}

	// Authorize the account with the application key.
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(b.cfg.APIURL, "/")+"/b2api/v3/b2_authorize_account", nil)
	if err != nil {
		return nil, err
	}
	httpReq.SetBasicAuth(b.cfg.KeyID, b.cfg.ApplicationKey)
	_, resp, err := b.do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "b2_authorize_account")
	}

	// Keep the account token and its API and download origins.
	token := string(resp.GetStringBytes("authorizationToken"))
	apiURL := string(resp.GetStringBytes("apiInfo", "storageApi", "apiUrl"))
	downloadURL := string(resp.GetStringBytes("apiInfo", "storageApi", "downloadUrl"))
	if token == "" || apiURL == "" || downloadURL == "" {
		return nil, errors.New("b2_authorize_account returned no token or URLs")
	}
	b.account = &b2Account{
		token:       token,
		apiURL:      strings.TrimSuffix(apiURL, "/"),
		downloadURL: downloadURL,
		expires:     now.Add(b2AccountTTL),
	}
	return b.account, nil
}

// call posts the JSON body with the account token and parses the answer.
func (b *B2) call(ctx context.Context, endpoint, token string, body []byte) (int, *fastjson.Value, error) {
	// Post the body with the account token.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	return b.do(req)
}

// do sends req and parses a 200 answer, returning the status.
func (b *B2) do(req *http.Request) (int, *fastjson.Value, error) {
	// Read a bounded answer.
	resp, err := b.cli.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}

	// Refuse a non-200 answer with its body, and parse the rest.
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, errors.Errorf("B2 answered %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	v, err := fastjson.ParseBytes(data)
	return resp.StatusCode, v, err
}

// B2 is a download authorizer.
var _ DownloadAuthorizer = (*B2)(nil)
