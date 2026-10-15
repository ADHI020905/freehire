package sources

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// mokahrFake answers the listing from trimmed real responses (2026-10-15), keyed by host and
// orgId: DeepSeek's REAL encrypted envelope (one posting), Moonshot's real plain body re-encrypted
// here the way app.mokahr.com sends it, and RedotPay's plain hire-r1 body. fullPages fills page 0
// with that many copies of the tenant's first row, to make the walk page.
type mokahrFake struct {
	t         *testing.T
	fullPages int
	failAt    int // offset whose request fails; -1 never
	asked     []string
}

func (f *mokahrFake) PostJSON(_ context.Context, u string, body, v any) error {
	q := body.(mokahrQuery)
	f.asked = append(f.asked, fmt.Sprintf("%s %s/%d %d+%d", u, q.OrgID, q.SiteID, q.Offset, q.Limit))
	if q.Offset == f.failAt {
		return errors.New("HTTP 502")
	}
	var file string
	switch {
	case u == "https://app.mokahr.com"+mokahrPath && q.OrgID == "high-flyer":
		return f.serve("deepseek_encrypted_limit1.json", v)
	case u == "https://app.mokahr.com"+mokahrPath && q.OrgID == "moonshot":
		file = "moonshot_plain.json"
	case u == "https://hire-r1.mokahr.com"+mokahrPath && q.OrgID == "redotpay":
		file = "redotpay_r1.json"
	default:
		return errors.New("unexpected POST " + u + " " + q.OrgID)
	}
	raw, err := os.ReadFile("testdata/mokahr/" + file)
	if err != nil {
		return err
	}
	var plain map[string]any
	if err := json.Unmarshal(raw, &plain); err != nil {
		f.t.Fatal(err)
	}
	data := plain["data"].(map[string]any)
	jobs := data["jobs"].([]any)
	if f.fullPages > 0 {
		if q.Offset >= f.fullPages*mokahrPageSize {
			jobs = nil
		} else {
			page := make([]any, 0, mokahrPageSize)
			for i := range mokahrPageSize {
				row := map[string]any{}
				for k, v := range jobs[0].(map[string]any) {
					row[k] = v
				}
				row["id"] = fmt.Sprintf("copy-%d-%d", q.Offset, i)
				page = append(page, row)
			}
			jobs = page
		}
	} else if q.Offset > 0 {
		jobs = nil
	}
	data["jobs"] = jobs
	out, _ := json.Marshal(plain)
	if q.OrgID == "moonshot" {
		out = mokahrTestEncrypt(f.t, out)
	}
	return json.Unmarshal(out, v)
}

func (f *mokahrFake) serve(file string, v any) error {
	raw, err := os.ReadFile("testdata/mokahr/" + file)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// mokahrTestEncrypt wraps a plain body the way app.mokahr.com does.
func mokahrTestEncrypt(t *testing.T, plain []byte) []byte {
	t.Helper()
	key := strings.Repeat("k", aes.BlockSize) // a test value, not a secret
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(pad)}, pad)...)
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, mokahrIV).CryptBlocks(ct, plain)
	out, _ := json.Marshal(map[string]string{"data": base64.StdEncoding.EncodeToString(ct), "necromancer": key})
	return out
}

func mokahrFetch(t *testing.T, f *mokahrFake, board, region string) ([]Job, error) {
	t.Helper()
	f.t = t
	return NewMokaHR(f).Fetch(context.Background(), CompanyEntry{Board: board, Region: region, Company: "Tenant"})
}

func TestMokaHRDecryptsTheRealEnvelope(t *testing.T) {
	jobs, err := mokahrFetch(t, &mokahrFake{failAt: -1}, "high-flyer/140576", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.Title == "" || !strings.HasPrefix(j.URL, "https://app.mokahr.com/social-recruitment/high-flyer/140576#/job/") ||
		!strings.Contains(j.Description, "<p>") || len(j.Countries) != 1 || j.Countries[0] != "cn" || j.PostedAt == nil {
		t.Errorf("decrypted posting: %+v", j)
	}
}

func TestMokaHRMapsATenant(t *testing.T) {
	jobs, err := mokahrFetch(t, &mokahrFake{failAt: -1}, "moonshot/148506", "")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (the paused posting is skipped)", len(jobs))
	}
	tech := byID["0079d23b-912d-4033-b9a4-22d7d970ba08"]
	if !tech.IsTechHint || tech.Location != "Beijing" || tech.EmploymentType != "full_time" {
		t.Errorf("technical function: %+v", tech)
	}
	if pr := byID["61bf7094-7ccf-4d1c-976d-b71393c7d52c"]; pr.IsTechHint {
		t.Errorf("a corporate function must not set the hint")
	}
	if bd := byID["7e79b93b-25ec-4545-88b4-09754ec9bb79"]; fmt.Sprint(bd.Countries) != "[cn us sg]" {
		t.Errorf("countries: %v", bd.Countries)
	}
}

func TestMokaHRReadsThePlainRegionalHost(t *testing.T) {
	jobs, err := mokahrFetch(t, &mokahrFake{failAt: -1}, "redotpay/100008889", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	// Hong Kong is country 中国 with province 香港.
	if j := jobs[0]; j.Location != "Hong Kong" || fmt.Sprint(j.Countries) != "[hk]" ||
		!strings.HasPrefix(j.URL, "https://hire-r1.mokahr.com/social-recruitment/redotpay/100008889#/job/") {
		t.Errorf("regional posting: %q %v %s", j.Location, j.Countries, j.URL)
	}
}

func TestMokaHRPagesToTheNaturalEnd(t *testing.T) {
	f := &mokahrFake{fullPages: 2, failAt: -1}
	jobs, err := mokahrFetch(t, f, "moonshot/148506", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2*mokahrPageSize || len(f.asked) != 3 {
		t.Fatalf("got %d jobs over %d requests", len(jobs), len(f.asked))
	}
}

func TestMokaHRFailsRatherThanTruncate(t *testing.T) {
	if _, err := mokahrFetch(t, &mokahrFake{fullPages: 2, failAt: mokahrPageSize}, "moonshot/148506", ""); err == nil {
		t.Error("a later page failing must fail the board")
	}
	for _, c := range []struct{ board, region string }{
		{"shopee/74378", ""}, {"moonshot", ""}, {"moonshot/148506", "eu"},
	} {
		if _, err := mokahrFetch(t, &mokahrFake{failAt: -1}, c.board, c.region); err == nil {
			t.Errorf("board %q region %q accepted", c.board, c.region)
		}
	}
}

func TestMokaHRRejectsAWrongKey(t *testing.T) {
	raw, err := os.ReadFile("testdata/mokahr/deepseek_encrypted_limit1.json")
	if err != nil {
		t.Fatal(err)
	}
	var env mokahrEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	env.Necromancer = "0000000000000000"
	if _, err := mokahrDecrypt(env); err == nil {
		t.Error("a wrong key must not decrypt")
	}
}
