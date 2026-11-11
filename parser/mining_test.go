package parser

import "testing"

func TestCriticalMining(t *testing.T) {
	message := "Critical mining success! You mined an additional 156 units of Loparite"
	events := ParseMining(message, "(mining)", 42)
	if len(events) != 1 {
		t.Fatalf("expected one critical bonus event: %+v", events)
	}
	event := events[0]
	if event.Kind != Mined || !event.Critical || event.Amount != 156 || event.Ore != "Loparite" || event.Second != 42 || event.Message != message {
		t.Fatalf("incorrect critical bonus: %+v", event)
	}
	regular := ParseMining("You mined 156 units of Loparite", "(mining)", 42)
	if len(regular) != 1 || regular[0].Critical {
		t.Fatalf("regular mining classified as critical: %+v", regular)
	}
	for _, invalid := range []string{
		"Critical mining success! You mined an additional -1 units of Loparite",
		"Critical mining success! You mined an additional 1,23 units of Loparite",
		"Critical mining success! You mined an additional 18446744073709551616 units of Loparite",
	} {
		if events := ParseMining(invalid, "(mining)", 42); len(events) != 0 {
			t.Errorf("accepted invalid bonus: %+v", events)
		}
	}
	if events := ParseMining(message, "(combat)", 42); len(events) != 0 {
		t.Fatalf("accepted critical outside mining channel: %+v", events)
	}
}
