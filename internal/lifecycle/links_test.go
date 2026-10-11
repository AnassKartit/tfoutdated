package lifecycle

import "testing"

func TestRegistryURL(t *testing.T) {
	tests := []struct {
		source, version, want string
	}{
		{"Azure/avm-ptn-virtualwan/azurerm", "", "https://registry.terraform.io/modules/Azure/avm-ptn-virtualwan/azurerm/latest"},
		{"terraform-aws-modules/vpc/aws", "6.7.3", "https://registry.terraform.io/modules/terraform-aws-modules/vpc/aws/6.7.3"},
		{"terraform-google-modules/network/google", "", "https://registry.terraform.io/modules/terraform-google-modules/network/google/latest"},
		{"Azure/avm-ptn-alz-connectivity-virtual-wan/azurerm//modules/virtual-wan", "", "https://registry.terraform.io/modules/Azure/avm-ptn-alz-connectivity-virtual-wan/azurerm/latest/submodules/virtual-wan"},
		{"git::https://github.com/cloudposse/terraform-null-ansible.git?ref=0.6.0", "", "https://github.com/cloudposse/terraform-null-ansible/tree/0.6.0"},
		{"git::https://github.com/org/repo.git//modules/net?ref=v1.2.0", "", "https://github.com/org/repo/tree/v1.2.0/modules/net"},
		{"git@github.com:org/repo.git", "", "https://github.com/org/repo"},
		{"github.com/org/repo", "", "https://github.com/org/repo"},
		{"./modules/local", "", ""},
		{"app.terraform.io/acme/vpc/aws", "", ""},
	}
	for _, tt := range tests {
		if got := RegistryURL(tt.source, tt.version); got != tt.want {
			t.Errorf("RegistryURL(%q, %q) = %q, want %q", tt.source, tt.version, got, tt.want)
		}
	}
}

func TestProviderURL(t *testing.T) {
	if got := ProviderURL("hashicorp/azurerm", "4.40.0"); got != "https://registry.terraform.io/providers/hashicorp/azurerm/4.40.0" {
		t.Errorf("got %q", got)
	}
	if got := ProviderURL("azurerm", ""); got != "" {
		t.Errorf("bare name should not link, got %q", got)
	}
}

// Every signal must come with a page that shows it, for any publisher, not
// only Azure Verified Modules.
func TestEvidenceLinksEverySignal(t *testing.T) {
	st := Status{
		RepoURL:       "https://github.com/terraform-aws-modules/terraform-aws-ebs-optimized",
		Archived:      true,
		ReadmeNotice:  true,
		ReleaseNotice: true,
		ReleaseTag:    "v2.0.0",
		MovedTo:       "terraform-aws-modules-archive/terraform-aws-ebs-optimized",
	}
	ev := st.evidence("terraform-aws-modules/ebs-optimized/aws", true, "")
	want := []string{
		"https://registry.terraform.io/modules/terraform-aws-modules/ebs-optimized/aws/latest",
		"https://github.com/terraform-aws-modules/terraform-aws-ebs-optimized/releases/tag/v2.0.0",
		"https://github.com/terraform-aws-modules/terraform-aws-ebs-optimized",
		"https://github.com/terraform-aws-modules-archive/terraform-aws-ebs-optimized",
	}
	if len(ev) != len(want) {
		t.Fatalf("evidence = %+v, want %d items", ev, len(want))
	}
	for i, e := range ev {
		if e.URL != want[i] || e.Label == "" {
			t.Errorf("evidence[%d] = %+v, want URL %q", i, e, want[i])
		}
	}
}
