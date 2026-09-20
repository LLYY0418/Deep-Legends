package main

import "testing"

func TestR90BQueueGuardRequired(t *testing.T) {
	for _, tc := range []struct{ name, declarations, body string }{
		{"channel-select", "", `ch:=make(chan int64,1);ch<-session.QueueID;select{case v:=<-ch:return v==1750}`},
		{"channel-receive", "", `ch:=make(chan int64,1);ch<-session.QueueID;v:=<-ch;return v==1750`},
		{"struct-field", "", `type wrap struct{q int64};w:=wrap{q:session.QueueID};return w.q==1750`},
		{"json-marshal", `import "encoding/json"`, `b,_:=json.Marshal(session.QueueID);return string(b)=="1750"`},
		{"binary-buffer", `import("encoding/binary";"bytes")`, `buf:=make([]byte,8);binary.BigEndian.PutUint64(buf,uint64(session.QueueID));target:=make([]byte,8);binary.BigEndian.PutUint64(target,1750);return bytes.Equal(buf,target)`},
		{"generic-comparator", `func genericEq[T comparable](a,b T)bool{return a==b}`, `return genericEq(session.QueueID,int64(1750))`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := r90QueueGuardSource(t, tc.declarations, tc.body)
			if len(arenaLiteralComparisons("champselect_fixture.go", source)) == 0 {
				t.Fatal("missed R90B queue literal:", tc.name)
			}
		})
	}
}

func TestR90BQueueGuardAllowsLegalFlows(t *testing.T) {
	for _, tc := range []struct{ name, declarations, body string }{
		{"channel-unrelated", "", `ch:=make(chan int64,1);ch<-session.ChampionID;v:=<-ch;return v==1750`},
		{"channel-registry", "", `ch:=make(chan int64,1);ch<-session.QueueID;v:=<-ch;return registeredQueueModeGroups[v]=="arena"`},
		{"struct-other-field", "", `type wrap struct{q,champ int64};w:=wrap{q:session.QueueID,champ:session.ChampionID};return w.champ==1750`},
		{"struct-other-type", "", `type first struct{q int64};w:=first{q:session.QueueID};_=w;type second struct{q int64};v:=second{q:session.ChampionID};return v.q==1750`},
		{"struct-registry", "", `type wrap struct{q int64};w:=wrap{q:session.QueueID};return registeredQueueModeGroups[w.q]=="arena"`},
		{"registry-metadata-pipeline", `
            type definition struct{Champions []int64}
            func classify(q int64)string{return registeredQueueModeGroups[q]}
            func lookup(group string)(definition,bool){return definition{Champions:[]int64{1}},true}
            func pool(d definition)([]int64,string){return d.Champions,"default"}
        `, `group:=classify(session.QueueID);def,_:=lookup(group);champions,_:=pool(def);return len(champions)==0`},
		{"json-unrelated", `import "encoding/json"`, `b,_:=json.Marshal(session.ChampionID);return string(b)=="1750"`},
		{"json-error-only", `import "encoding/json"`, `_,err:=json.Marshal(session.QueueID);return err==nil`},
		{"binary-unrelated", `import("encoding/binary";"bytes")`, `buf:=make([]byte,8);binary.BigEndian.PutUint64(buf,uint64(session.ChampionID));target:=make([]byte,8);binary.BigEndian.PutUint64(target,1750);return bytes.Equal(buf,target)`},
		{"generic-unrelated", `func genericEq[T comparable](a,b T)bool{return a==b}`, `return genericEq(session.ChampionID,int64(1750))`},
		{"generic-registry", `func genericEq[T comparable](a,b T)bool{return a==b}`, `return genericEq(registeredQueueModeGroups[session.QueueID],"arena")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := r90QueueGuardSource(t, tc.declarations, tc.body)
			if found := arenaLiteralComparisons("champselect_fixture.go", source); len(found) != 0 {
				t.Fatalf("legal R90B flow rejected: %s: %v", tc.name, found)
			}
		})
	}
}

// Variants of the same six data-flow paths, not claims of new analysis classes.
func TestR90BQueueGuardFlowVariants(t *testing.T) {
	for _, tc := range []struct{ name, declarations, body string }{
		{"channel-comma-ok", "", `ch:=make(chan int64,1);ch<-session.QueueID;v,ok:=<-ch;return ok&&v==1750`},
		{"channel-numeric-target", "", `ch:=make(chan int64,1);ch<-int64(1750);v:=<-ch;return session.QueueID==v`},
		{"struct-positional-alias", "", `type wrap struct{q int64};w:=wrap{session.QueueID};alias:=w;return alias.q==1750`},
		{"struct-pointer", "", `type wrap struct{q int64};w:=&wrap{q:session.QueueID};return w.q==1750`},
		{"json-function-alias", `import j "encoding/json"`, `encode:=j.Marshal;var b,err=encode(session.QueueID);return err==nil&&string(b)=="1750"`},
		{"three-results-first-value", `func values(q int64)(int64,string,error){return q,"",nil}`, `first,_,_:=values(session.QueueID);return first==1750`},
		{"binary-compare-alias", `import(b "encoding/binary";"bytes")`, `buf:=make([]byte,4);write:=b.LittleEndian.PutUint32;write(buf,uint32(session.QueueID));target:=make([]byte,4);write(target,1750);return bytes.Compare(buf,target)==0`},
		{"generic-function-alias", `func genericEq[T comparable](a,b T)bool{return a==b}`, `equal:=genericEq[int64];return equal(session.QueueID,1750)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := r90QueueGuardSource(t, tc.declarations, tc.body)
			if len(arenaLiteralComparisons("champselect_fixture.go", source)) == 0 {
				t.Fatal("missed R90B flow variant:", tc.name)
			}
		})
	}
}
