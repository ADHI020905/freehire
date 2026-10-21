package collections

import (
	"reflect"
	"testing"
)

func TestParseDKSponsors_ReadsThePastedSpreadsheetTable(t *testing.T) {
	// The live page's shape: a spreadsheet pasted into the CMS, every cell's text in
	// a <p>, the header row made of <h4>s, and stray &nbsp; padding.
	html := `
	<table width="614">
	  <tr height="19"><td class="xl65"><h4 class="black h3">Name of company</h4></td><td class="xl66"><h4>CVR no.</h4></td></tr>
	  <tr height="19">
	    <td class="xl64"><p class="bodycopy-small">&amp;TRADITION A/S</p></td>
	    <td class="xl65"><p class="bodycopy-small">18169304</p></td>
	  </tr>
	  <tr height="19">
	    <td class="xl63"><p class="bodycopy-small">3Shape A/S&nbsp;</p></td>
	    <td class="xl66"><p class="bodycopy-small">25553489&nbsp;</p></td>
	  </tr>
	</table>`

	got, err := ParseDKSponsors([]byte(html))
	if err != nil {
		t.Fatalf("ParseDKSponsors: %v", err)
	}
	want := []Record{
		{Name: "&TRADITION A/S", Meta: map[string]string{"cvr": "18169304"}},
		{Name: "3Shape A/S", Meta: map[string]string{"cvr": "25553489"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseDKSponsors =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseDKSponsors_RejectsAPageWithNoRows(t *testing.T) {
	if _, err := ParseDKSponsors([]byte(`<p>Page not found</p>`)); err == nil {
		t.Fatal("ParseDKSponsors accepted a page with no table rows")
	}
}
