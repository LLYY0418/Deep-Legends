package main

// This is a reviewed first-team roster, NOT the OP.GG team membership list.
// OP.GG includes coaches, academy teams and historical team assignments. Keep
// roster changes reviewable; sources and unresolved candidates are in
// docs/pro-players-sources.md. Production directory rows and badges both use
// these six reviewed teams; proDirectoryRoster is reserved for future expansion.
// Exact PUUID/full Riot ID lookup is allowed;
// never infer membership from an account name substring or team prefix.
const proRosterVerifiedAt = "2026-09-08"

type proRosterPlayer struct {
	Directory  bool  // Source-reported, not part of the reviewed roster.
	AllowTeams []int // Explicit reviewed historical upstream team exceptions.
	Name       string
	Position   string
	Names      []string // verified real-name spellings, including native-script names
}

type proRosterTeam struct {
	Code    string
	Name    string
	League  string
	OPGGID  int
	Players []proRosterPlayer
}

var proRoster = []proRosterTeam{
	{"BLG", "Bilibili Gaming", "LPL", 632, []proRosterPlayer{
		{Name: "Bin", Position: "top", Names: []string{"Chen Ze-Bin", "陈泽彬"}},
		{Name: "Wenbo", Position: "top", Names: []string{"Yang Wenbo", "杨文波"}},
		{Name: "Flandre", Position: "top", Names: []string{"Li Xuan-Jun", "李炫君"}},
		{Name: "Xun", Position: "jungle", Names: []string{"Peng Lixun", "彭立勋"}},
		{Name: "knight", Position: "middle", Names: []string{"Zhuo Ding", "卓定"}},
		{Name: "Viper", Position: "bottom", Names: []string{"Park Do-hyeon", "박도현"}},
		{Name: "ON", Position: "utility", Names: []string{"Luo Wen-Jun", "骆文俊"}},
	}},
	{"IG", "Invictus Gaming", "LPL", 371, []proRosterPlayer{
		{Name: "TheShy", Position: "top", Names: []string{"Kang Seung-lok", "강승록", "姜承録"}},
		{Name: "Wei", Position: "jungle", Names: []string{"Yan Yang-Wei", "闫扬威"}},
		{AllowTeams: []int{858}, Name: "Rookie", Position: "middle", Names: []string{"Song Eui-jin", "송의진"}},
		{Name: "Assum", Position: "bottom", Names: []string{"Zou Wei", "邹维"}},
		{Name: "JiaQi", Position: "bottom", Names: []string{"Zi Jiaqi", "资嘉琪"}},
		{AllowTeams: []int{415}, Name: "Meiko", Position: "utility", Names: []string{"Tian Ye", "田野"}},
	}},
	{"T1", "T1", "LCK", 385, []proRosterPlayer{
		{Name: "Doran", Position: "top", Names: []string{"Choi Hyeon-jun", "Choi Hyeon-joon", "최현준"}},
		{Name: "Oner", Position: "jungle", Names: []string{"Mun Hyeon-jun", "Moon Hyeon-joon", "문현준"}},
		{Name: "Faker", Position: "middle", Names: []string{"Lee Sang-hyeok", "이상혁"}},
		{Name: "Peyz", Position: "bottom", Names: []string{"Kim Su-hwan", "김수환"}},
		{Name: "Keria", Position: "utility", Names: []string{"Ryu Min-seok", "류민석"}},
	}},
	{"HLE", "Hanwha Life Esports", "LCK", 591, []proRosterPlayer{
		{Name: "Zeus", Position: "top", Names: []string{"Choi Woo-je", "최우제"}},
		{Name: "Kanavi", Position: "jungle", Names: []string{"Seo Jin-hyeok", "서진혁"}},
		{Name: "Zeka", Position: "middle", Names: []string{"Kim Geon-woo", "김건우"}},
		{Name: "Gumayusi", Position: "bottom", Names: []string{"Lee Min-hyeong", "이민형"}},
		{Name: "Delight", Position: "utility", Names: []string{"Yoo Hwan-jung", "Yoo Hwan-joong", "유환중"}},
	}},
	{"GEN", "Gen.G", "LCK", 416, []proRosterPlayer{
		{Name: "Kiin", Position: "top", Names: []string{"Kim Gi-in", "김기인"}},
		{Name: "Canyon", Position: "jungle", Names: []string{"Kim Geon-bu", "김건부"}},
		{Name: "Chovy", Position: "middle", Names: []string{"Jeong Ji-hoon", "정지훈"}},
		{Name: "Ruler", Position: "bottom", Names: []string{"Park Jae-hyeok", "Park Jae-hyuk", "박재혁"}},
		{Name: "Duro", Position: "utility", Names: []string{"Joo Min-kyu", "주민규"}},
	}},
	{"DK", "Dplus KIA", "LCK", 1681, []proRosterPlayer{
		{Name: "Siwoo", Position: "top", Names: []string{"Jeon Si-woo", "전시우"}},
		{Name: "Lucid", Position: "jungle", Names: []string{"Choi Yong-hyeok", "최용혁"}},
		{Name: "ShowMaker", Position: "middle", Names: []string{"Heo Su", "허수"}},
		{Name: "Smash", Position: "bottom", Names: []string{"Shin Geum-jae", "신금재"}},
		{Name: "Career", Position: "utility", Names: []string{"Oh Hyeong-seok", "Oh Hyung-seok", "오형석"}},
	}},
}
