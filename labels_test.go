package main

import (
	"strings"
	"testing"
)

func TestLabelPatches(t *testing.T) {
	patch, err := labelEditPatch(strings.NewReader(`{"LabelID":"7","Name":"Customer","Color":3,"Deleted":false}`))
	if err != nil {
		t.Fatal(err)
	}
	action := patch.Mutations[0].Value.GetLabelEditAction()
	if action.GetName() != "Customer" || action.GetColor() != 3 || action.GetDeleted() || patch.Mutations[0].Index[1] != "7" {
		t.Fatal("incorrect edit")
	}
	patch, err = labelEditPatch(strings.NewReader(`{"LabelID":"7","Deleted":true}`))
	if err != nil || !patch.Mutations[0].Value.GetLabelEditAction().GetDeleted() {
		t.Fatal("delete failed", err)
	}
	for _, message := range []bool{false, true} {
		for _, labeled := range []string{"true", "false"} {
			body := `{"Chat":"123@g.us","LabelID":"7","Labeled":` + labeled
			if message {
				body += `,"Id":"ABC"`
			}
			body += `}`
			patch, err := labelAssociationPatch(message)(strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			m := patch.Mutations[0]
			if m.Value.GetLabelAssociationAction().GetLabeled() != (labeled == "true") || m.Index[1] != "7" || m.Index[2] != "123@g.us" {
				t.Fatal("incorrect association")
			}
			if message && m.Index[3] != "ABC" {
				t.Fatal("incorrect message ID")
			}
		}
	}
	for _, body := range []string{`{}`, `{"LabelID":"-1","Deleted":true}`, `{"LabelID":"01","Deleted":true}`, `{"LabelID":"4294967296","Deleted":true}`, `{"LabelID":"7","Deleted":false}`, `{"LabelID":"7","Name":"X","Color":20,"Deleted":false}`, `{"LabelID":"7","Name":" ","Color":1,"Deleted":false}`} {
		if _, err := labelEditPatch(strings.NewReader(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	for _, body := range []string{`{"Chat":"123@g.us","LabelID":"7"}`, `{"Chat":"123@g.us","LabelID":"bad","Labeled":true}`, `{"Chat":"123@g.us","LabelID":"7","Labeled":true}`} {
		if _, err := labelAssociationPatch(true)(strings.NewReader(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestLabelRoutes(t *testing.T) {
	testFeatureRoute(t, "/labels/edit", "labels.edit", `{"LabelID":"7","Name":"Customer","Color":3,"Deleted":false}`)
	testFeatureRoute(t, "/labels/chat", "labels.chat", `{"Chat":"123@g.us","LabelID":"7","Labeled":true}`)
	testFeatureRoute(t, "/labels/message", "labels.message", `{"Chat":"123@g.us","LabelID":"7","Id":"ABC","Labeled":false}`)
}
