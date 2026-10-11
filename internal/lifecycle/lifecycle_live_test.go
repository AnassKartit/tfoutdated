package lifecycle

// Live tests: they query the real Terraform Registry, GitHub and the AVM
// module index, so they only run when TFOUTDATED_LIVE=1. Set GH_TOKEN (for
// example GH_TOKEN=$(gh auth token)) for the full sweep, which needs more than
// GitHub's 60 unauthenticated requests per hour.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/anasskartit/tfoutdated/internal/httpx"
)

func skipUnlessLive(t *testing.T) {
	t.Helper()
	if os.Getenv("TFOUTDATED_LIVE") != "1" {
		t.Skip("set TFOUTDATED_LIVE=1 to run live checks")
	}
}

type registryModule struct {
	Source string `json:"source"`
	Root   struct {
		Readme string `json:"readme"`
	} `json:"root"`
}

// fetchRegistry returns what the resolver gets from the registry for a module:
// its repository URL and the README of the latest version. found is false for
// modules not published to the registry.
func fetchRegistry(ctx context.Context, client *httpx.Client, source string) (mod registryModule, found bool, err error) {
	resp, err := client.Get(ctx, "https://registry.terraform.io/v1/modules/"+source,
		map[string]string{"Accept": "application/json"})
	if err != nil {
		return mod, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return mod, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return mod, false, fmt.Errorf("registry %s: HTTP %d", source, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return mod, false, err
	}
	return mod, true, json.Unmarshal(body, &mod)
}

// registrySource returns "Azure/<name>/<provider>" for an AVM module name, using
// the provider it is actually published under: most use azurerm, some use
// azure or azapi. found is false when the module is not published.
func registrySource(ctx context.Context, client *httpx.Client, name string) (string, bool, error) {
	resp, err := client.Get(ctx, "https://registry.terraform.io/v1/modules/Azure/"+name,
		map[string]string{"Accept": "application/json"})
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	var list struct {
		Modules []struct {
			Provider string `json:"provider"`
		} `json:"modules"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return "", false, err
	}
	if len(list.Modules) == 0 {
		return "", false, nil
	}
	return "Azure/" + name + "/" + list.Modules[0].Provider, true, nil
}

// The two modules raised in review: deprecated in their v0.14.1 / v0.10.3
// releases, archived on GitHub, README pointing to a submodule elsewhere.
func TestLiveKnownDeprecatedModules(t *testing.T) {
	skipUnlessLive(t)
	ctx := context.Background()
	c := New(Options{})
	client := httpx.New()

	cases := []struct {
		source      string
		replacement string // expected prefix of ReplacedBy
	}{
		{"Azure/avm-ptn-virtualwan/azurerm", "Azure/avm-ptn-alz-connectivity-virtual-wan/azurerm//modules/"},
		{"Azure/avm-ptn-vnetgateway/azurerm", "Azure/avm-ptn-alz-connectivity-hub-and-spoke-vnet/azurerm//modules/"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			reg, found, err := fetchRegistry(ctx, client, tc.source)
			if err != nil || !found {
				t.Fatalf("registry lookup: found=%v err=%v", found, err)
			}
			st, err := c.Check(ctx, tc.source, reg.Source, reg.Root.Readme)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			t.Logf("%s: %s | replaced by %s", tc.source, st.Message(), st.ReplacedBy)

			if !st.ReadmeNotice {
				t.Error("README of the latest published version should announce deprecation")
			}
			if !st.ReleaseNotice {
				t.Error("latest release notes should announce deprecation")
			}
			if !st.Archived {
				t.Error("GitHub repository should be archived")
			}
			if st.AVMStatus != "Deprecated" {
				t.Errorf("AVM index status = %q, want Deprecated", st.AVMStatus)
			}
			if !strings.HasPrefix(st.ReplacedBy, tc.replacement) {
				t.Errorf("ReplacedBy = %q, want prefix %q", st.ReplacedBy, tc.replacement)
			}
		})
	}
}

// Sweeps every module in the AVM Terraform indexes. It fails only when a module
// Microsoft marks Deprecated goes undetected, and logs everything flagged
// (deprecated, archived, moved) so the run doubles as a report.
func TestLiveAllAVMModules(t *testing.T) {
	skipUnlessLive(t)
	if os.Getenv("GH_TOKEN") == "" && os.Getenv("GITHUB_TOKEN") == "" && os.Getenv("TFOUTDATED_GITHUB_TOKEN") == "" {
		t.Skip("set GH_TOKEN for the full sweep (hundreds of GitHub calls)")
	}
	ctx := context.Background()
	c := New(Options{})
	client := httpx.New()

	c.avmEntry(ctx, "") // load the index
	if c.avmErr != nil {
		t.Fatalf("loading AVM index: %v", c.avmErr)
	}
	names := make([]string, 0, len(c.avm))
	for name := range c.avm {
		names = append(names, name)
	}
	sort.Strings(names)

	type row struct {
		name   string
		status Status
		err    error
	}
	var (
		mu          sync.Mutex
		rows        []row
		unpublished int
		wg          sync.WaitGroup
		sem         = make(chan struct{}, 8)
	)
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			source, found, err := registrySource(ctx, client, name)
			var reg registryModule
			if err == nil && found {
				reg, found, err = fetchRegistry(ctx, client, source)
			}
			if err == nil && !found {
				mu.Lock()
				unpublished++
				mu.Unlock()
				return
			}
			var st Status
			if err == nil {
				st, err = c.Check(ctx, source, reg.Source, reg.Root.Readme)
			}
			mu.Lock()
			rows = append(rows, row{name, st, err})
			mu.Unlock()
		}(name)
	}
	wg.Wait()
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	var deprecated, archived, moved, flagged, errs int
	for _, r := range rows {
		s := r.status
		if s.AVMStatus == "Deprecated" {
			deprecated++
		}
		if s.Archived {
			archived++
		}
		if s.MovedTo != "" {
			moved++
		}
		if r.err != nil {
			errs++
			t.Logf("ERROR %-55s %v", r.name, r.err)
		}
		if s.NeedsAttention() {
			flagged++
			t.Logf("%-55s index=%-10s %s | replaced by: %s", r.name, s.AVMStatus, s.Message(), orDash(s.ReplacedBy))
		}
		// The point of the feature: nothing Microsoft deprecated slips through.
		if s.AVMStatus == "Deprecated" && r.err == nil && !s.Stopped() {
			t.Errorf("%s is Deprecated in the AVM index but was not detected", r.name)
		}
	}
	t.Logf("checked %d published modules (%d in index, %d not published): %d flagged, %d deprecated in index, %d archived, %d moved, %d errors",
		len(rows), len(names), unpublished, flagged, deprecated, archived, moved, errs)
}

// Known cases outside the AVM catalogue: older Azure modules, Azure CAF, AWS
// and modules whose repository was transferred. Logged as a report; the
// expectations below are what GitHub and the registry showed when written.
func TestLiveOtherKnownModules(t *testing.T) {
	skipUnlessLive(t)
	ctx := context.Background()
	c := New(Options{})
	client := httpx.New()

	sources := []string{
		// Azure, pre-AVM and CAF
		"Azure/compute/azurerm",
		"Azure/network/azurerm",
		"Azure/vnet/azurerm",
		"Azure/aks/azurerm",
		"Azure/caf-enterprise-scale/azurerm",
		"aztfmod/caf/azurerm",
		// AWS
		"terraform-aws-modules/ebs-optimized/aws",
		"terraform-aws-modules/s3-object/aws",
		"terraform-aws-modules/vpc/aws", // active: must stay clean
		// transferred to another organisation
		"cloudposse/ansible/null",
		"cloudposse/iam-chamber-s3-role/aws",
	}
	for _, source := range sources {
		reg, found, err := fetchRegistry(ctx, client, source)
		if err != nil || !found {
			t.Logf("%-45s not in registry (found=%v err=%v)", source, found, err)
			continue
		}
		st, err := c.Check(ctx, source, reg.Source, reg.Root.Readme)
		verdict := "ok"
		if st.NeedsAttention() {
			verdict = st.Message()
		}
		t.Logf("%-45s %s | replaced by: %s | err: %v", source, verdict, orDash(st.ReplacedBy), err)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
