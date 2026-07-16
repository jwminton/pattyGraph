package main

import "testing"

func TestFactoidSchedulesPreserveCooldowns(t *testing.T) {
	random := RandomSchedule(100)
	repeating := RepeatingSchedule(100)

	if random(10, 1, false) {
		t.Fatal("RandomSchedule should wait ten cycles after lastSeen")
	}
	if !random(11, 1, false) {
		t.Fatal("RandomSchedule should allow display after ten cycles")
	}

	if repeating(2, 1, false) {
		t.Fatal("RepeatingSchedule should wait more than one cycle after lastSeen")
	}
	if !repeating(3, 1, false) {
		t.Fatal("RepeatingSchedule should allow display after two cycles")
	}
}

func TestScheduledUsesSingleConstructorModel(t *testing.T) {
	f := Scheduled(42, AlwaysSchedule(), func(_ []string) string {
		return "ok"
	})

	if f.probability != 42 {
		t.Fatalf("probability = %d, want 42", f.probability)
	}
	if !f.Condition(1, 0, false) {
		t.Fatal("AlwaysSchedule condition should be true")
	}
	if got := f.Generate(nil); got != "ok" {
		t.Fatalf("Generate() = %q, want ok", got)
	}
}

func TestDirectOnlyFactNeverEntersBackgroundSchedule(t *testing.T) {
	f := DirectOnly(func(_ []string) string { return "direct" })

	if f.probability != 100 {
		t.Fatalf("probability = %d, want 100", f.probability)
	}
	if f.Condition(1, 0, false) || f.Condition(100, 0, true) {
		t.Fatal("DirectOnly condition should always be false")
	}
}
