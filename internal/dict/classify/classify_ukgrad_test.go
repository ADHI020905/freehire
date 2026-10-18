package classify

import "testing"

// TestParseUKEarlyCareers covers the UK graduate-market vocabulary gradcracker lists
// (most titles copied from its live listing): a placement is an internship, a graduate
// scheme is the entry-level hire, and the university offices that merely contain the
// word "graduate" state no grade at all (those titles are copied from prod).
func TestParseUKEarlyCareers(t *testing.T) {
	cases := []struct {
		title         string
		wantSeniority string
	}{
		{"Graduate Software Engineer", "junior"},
		{"2027 Graduate - Software & AI Systems Engineer", "junior"},
		{"IMS Technology - Software Engineering Graduate Scheme", "junior"},
		{"Technology Graduate", "junior"},
		{"RN I New Graduate Resident", "junior"},
		{"Software Engineering Industrial Placement", "intern"},
		{"Information Technology Industrial Placement", "intern"},
		{"Student Placement - Information Technology", "intern"},
		{"Systems Engineer - Summer Placement 2027", "intern"},
		{"Product Cyber Security - Undergraduate Placement 2027", "intern"},
		{"Global Technology, Technology Infrastructure 12 Month Industrial Placement 2027 Analyst", "intern"},
		{"Software Engineer 12-month placement", "intern"},
		{"Technology Intern - Year in Industry", "intern"},
		{"Summer Vacation Scheme - Forensic Technology", "intern"},
		{"Technology Summer Analyst", "intern"},
		{"IT Placement Student", "intern"},
		// The weakest evidence loses to every other grade the title states.
		{"Graduate Summer Internship - Software", "intern"},
		{"Senior Graduate Engineer", "senior"},
		// A bare placement or undergraduate states no grade.
		{"Placement Coordinator", ""},
		{"The Undergraduate School - Adjunct Faculty", ""},
		{"Undergraduate Software Engineer", ""},
		// "postgraduate" is one token, not the word "graduate".
		{"Postgraduate Research Associate", ""},
		// University offices and the graduate-recruiting desk name no grade.
		{"Assistant Director of Graduate Admissions", ""},
		{"Graduate Admissions Coordinator", ""},
		{"Assistant Manager, Graduate School", ""},
		{"FWS Student Assistant - Office of Graduate Studies", ""},
		{"Director, McDougal Graduate Student Center and Student Life", ""},
		{"Graduate Recruitment Manager", ""},
		{"Assoc Professor-Graduate Program Coordinator-FWWSON", ""},
		// The staff who RUN graduate programmes and placements: the word sits next to the
		// desk it names, and that desk states no grade.
		{"Director of Graduate Programs", ""},
		{"Graduate Director", ""},
		{"Graduate Program Manager", ""},
		{"Manager, Graduate Programs", ""},
		{"Graduate Recruiting Manager", ""},
		{"Graduate Coordinator", ""},
		{"Graduate Advisor", ""},
		{"Graduate Medical Education Coordinator", ""},
		{"Work Placement Officer", ""},
		{"Industrial Placement Coordinator", ""},
		{"Placement Year Manager", ""},
		// ...while the hires themselves keep their grade.
		{"Senior Graduate Recruiter", "senior"},
		{"Industrial Placement – Software", "intern"},
		{"Graduate Project Manager", "junior"},
		{"Software Engineering Graduate Programme", "junior"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Seniority; got != c.wantSeniority {
			t.Errorf("Parse(%q).Seniority = %q, want %q", c.title, got, c.wantSeniority)
		}
	}
}
