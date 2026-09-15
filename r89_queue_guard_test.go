package main

import "testing"

func TestR89QueueGuardEquivalentMutations(t *testing.T) {
	for name, body := range map[string]string{
		"string-direct":      `return strconv.FormatInt(session.QueueID, 10) == "3110"`,
		"string-alias":       `q := session.QueueID; text := strconv.FormatInt(q,10); alias := text; return alias != "1700"`,
		"itoa":               `return strconv.Itoa(int(session.QueueID)) == "3110"`,
		"format":             `return fmt.Sprintf("%d",session.QueueID) == "3110"`,
		"sprint":             `return fmt.Sprint(session.QueueID) == "3110"`,
		"format-alias":       `format := strconv.FormatInt; return format(session.QueueID,10) == "3110"`,
		"string-constant":    `const code = "31"+"10"; return strconv.FormatInt(session.QueueID,10) == code`,
		"constant-alias":     `const code = 3110; alias := code; return session.QueueID == alias`,
		"reassigned-literal": `code := "arena"; code = "3110"; alias := code; return fmt.Sprint(session.QueueID) == alias`,
		"float-conversion":   `return float64(session.QueueID) == 3110.0`,
		"float-exponent":     `return float64(session.QueueID) == 3.11e3`,
		"cast-literal":       `return float64(session.QueueID) == float64(3110)`,
		"unsigned-format":    `return strconv.FormatUint(uint64(session.QueueID),10) == "3110"`,
		"numeric-string-rhs": `return strconv.FormatInt(session.QueueID,10) == strconv.Itoa(3110)`,
		"wrapper":            `return identity(session.QueueID) == 3110`,
		"wrapper-extra-args": `return identity(ctx,session.QueueID) == 3110`,
		"receiver-string":    `return session.QueueID.String() == "3110"`,
		"arithmetic":         `q := session.QueueID + 0; return q == 3100+10`,
		"shift-literal":      `return session.QueueID == 850<<1`,
		"reflect":            `return reflect.DeepEqual(session.QueueID,int64(3110))`,
		"reflect-string":     `return reflect.DeepEqual(fmt.Sprint(session.QueueID),"3110")`,
		"reflect-alias":      `equal := reflect.DeepEqual; other := equal; return other(session.QueueID,3110)`,
		"equal-fold":         `return strings.EqualFold(fmt.Sprint(session.QueueID),"3110")`,
		"compare":            `return strings.Compare(fmt.Sprint(session.QueueID),"3110") == 0`,
		"generic-compare":    `return cmp.Compare(session.QueueID,3110) == 0`,
		"bytes":              `return bytes.Equal([]byte(fmt.Sprint(session.QueueID)),[]byte("3110"))`,
		"string-switch":      `switch fmt.Sprintf("%d",session.QueueID) {case "1700","3110":return true}; return false`,
		"constant-switch":    `const code = 3110; switch session.QueueID {case code:return true};return false`,
		"original-direct":    `return session.QueueID == 3110`,
		"original-alias":     `q:=session.QueueID;return q==1700`,
		"original-switch":    `switch session.QueueID {case 1700,1710:return true};return false`,
		"named-queue-root":   `queueID:=3110; return queueID==3110`,
	} {
		t.Run(name, func(t *testing.T) {
			source := `package main; import("strconv";"fmt";"reflect";"strings";"bytes";"cmp"); func f(session struct{QueueID int64}) bool {` + body + `}`
			if len(arenaLiteralComparisons("mutant.go", source)) == 0 {
				t.Fatal("missed equivalent queue literal comparison:", body)
			}
		})
	}
	if len(arenaLiteralComparisons("import-alias.go", `package main;import r "reflect";import s "strconv";func f(queueID int64)bool{return r.DeepEqual(s.FormatInt(queueID,10),"3110")}`)) == 0 {
		t.Fatal("import aliases bypass the guard")
	}
}

func TestR89QueueGuardAllowsRegisteredTablesAndUnrelatedValues(t *testing.T) {
	for name, body := range map[string]string{
		"registered-table":     `return registeredQueueModeGroups[session.QueueID] == "arena"`,
		"table-result-numeric": `return registeredQueueProperties[session.QueueID].Size == 3`,
		"table-lookup-local":   `value, ok := registeredQueueModeGroups[session.QueueID]; return ok && value == "arena"`,
		"shared-classifier":    `return queueModeGroupFor(session.QueueID, "", 0) == "arena"`,
		"queue-identity":       `return session.QueueID == previous.QueueID`,
		"formatted-identity":   `return strconv.FormatInt(session.QueueID,10) == strconv.FormatInt(previous.QueueID,10)`,
		"other-identity":       `return strconv.FormatInt(session.QueueID,10) == strconv.FormatInt(otherID,10)`,
		"empty-string":         `return fmt.Sprint(session.QueueID) == ""`,
		"nonnumeric-string":    `return fmt.Sprint(session.QueueID) == "arena"`,
		"unrelated-shadow":     `q:=session.QueueID;_=q;{q:=1;return q==1}`,
		"unrelated-constant":   `return session.ChampionID == 3110`,
	} {
		t.Run(name, func(t *testing.T) {
			source := `package main; import("strconv";"fmt"); func f(session struct{QueueID,ChampionID int64}) bool {` + body + `}`
			if got := arenaLiteralComparisons("valid.go", source); len(got) != 0 {
				t.Fatalf("legal expression rejected: %s: %v", body, got)
			}
		})
	}
}
