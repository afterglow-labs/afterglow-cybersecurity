package db

import "testing"

func TestProfileAccessProgramSurvivesCredentialUpdates(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	p := &LLMProfile{Name: "Blue persistence regression", Format: "openai-responses", Model: "gpt-6-sol", APIKey: "test-key", AccessProgramCyber: "daybreak_blue"}
	id, err := d.SaveProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM llm_profiles WHERE id=$1`, id) })
	stored, err := d.ProfileByID(id)
	if err != nil || stored == nil || stored.AccessProgramCyber != "daybreak_blue" {
		t.Fatalf("new profile lost access program: %+v %v", stored, err)
	}
	for _, step := range []struct{ program, key string }{
		{"daybreak_blue", "test-key"}, {"standard", ""}, {"daybreak_blue", "replacement-key"},
	} {
		p.ID, p.AccessProgramCyber, p.APIKey = id, step.program, step.key
		if _, err = d.SaveProfile(p); err != nil {
			t.Fatal(err)
		}
		stored, err := d.ProfileByID(id)
		if err != nil || stored == nil {
			t.Fatalf("load: %v", err)
		}
		if stored.AccessProgramCyber != step.program {
			t.Fatalf("program=%q want %q", stored.AccessProgramCyber, step.program)
		}
		if step.key == "" && stored.APIKey != "test-key" {
			t.Fatal("updating the flag changed the retained key")
		}
	}
	p.Format, p.APIKey = "ollama", ""
	if _, err = d.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	stored, err = d.ProfileByID(id)
	if err != nil || stored.AccessProgramCyber != "" {
		t.Fatal("non-Responses profile retained a Daybreak flag", err)
	}
}
