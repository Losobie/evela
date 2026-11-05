package main

const filterTarget = "Target"
const filterTargetType = "Target Type"

func targetGroupingMode(mode string) bool { return mode == filterTarget || mode == filterTargetType }

func targetValue(name, typeName, kind string) string {
	if kind == filterTargetType {
		return typeName
	}
	return name
}
