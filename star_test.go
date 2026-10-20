package main

import (
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestStarRequest(t *testing.T) {
	for _, star := range []bool{true, false} {
		r := starRequest{Chat: "123@g.us", Sender: "456@lid", Id: "ABC", FromMe: proto.Bool(false), Star: proto.Bool(star)}
		patch, err := r.patch()
		if err != nil {
			t.Fatal(err)
		}
		mutation := patch.Mutations[0]
		if mutation.Value.GetStarAction().GetStarred() != star {
			t.Fatal("incorrect star action")
		}
		for _, invalid := range []starRequest{
			{Chat: r.Chat, Id: r.Id, FromMe: r.FromMe, Star: r.Star},
			{Chat: r.Chat, Sender: r.Sender, FromMe: r.FromMe, Star: r.Star},
			{Chat: r.Chat, Sender: r.Sender, Id: r.Id, Star: r.Star},
			{Chat: r.Chat, Sender: r.Sender, Id: r.Id, FromMe: r.FromMe},
		} {
			if _, err := invalid.patch(); err == nil {
				t.Fatalf("accepted invalid request: %+v", invalid)
			}
		}
	}
}
