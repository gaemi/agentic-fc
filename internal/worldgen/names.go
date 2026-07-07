package worldgen

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Name generation for the four v1 cultures (docs/09 §4.1, §6). Pools are
// combinatorial (prefix × suffix) so even a Sprawling world never runs dry;
// a registry rerolls collisions deterministically. East Asian names are
// romanized, family-name-first, and drawn from three sub-pools (KR/JP/CN)
// that never cross-mix.

// pickCulture samples the configured mix in AllCultures order.
func pickCulture(r *rand.Rand, mix CultureMix) Culture {
	total := 0
	for _, w := range mix {
		total += w
	}
	roll := r.IntN(total)
	for i, w := range mix {
		if roll < w {
			return AllCultures[i]
		}
		roll -= w
	}
	return CultureAnglo // unreachable with a valid mix
}

// nameRegistry keeps generated names unique where uniqueness matters
// (places, club names, short names).
type nameRegistry struct{ used map[string]bool }

func newNameRegistry() *nameRegistry { return &nameRegistry{used: map[string]bool{}} }

// claim rerolls gen() until it produces an unused name; after enough
// collisions it appends a Roman-ish numeral suffix to force uniqueness.
func (nr *nameRegistry) claim(gen func() string) string {
	for i := 0; i < 64; i++ {
		n := gen()
		if !nr.used[n] {
			nr.used[n] = true
			return n
		}
	}
	base := gen()
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s %d", base, i)
		if !nr.used[n] {
			nr.used[n] = true
			return n
		}
	}
}

// ---- Place names ----

var angloPlacePrefix = []string{
	"Alder", "Ash", "Barrow", "Beck", "Birch", "Black", "Bright", "Brook",
	"Castle", "Clay", "Cross", "Dun", "East", "Elder", "Fair", "Fen", "Ford",
	"Gold", "Green", "Grey", "Hart", "Haven", "Hazel", "High", "Holly",
	"Iron", "Kings", "Lang", "Mere", "Mill", "Moor", "North", "Oak", "Old",
	"Ox", "Queens", "Raven", "Red", "Rose", "Rush", "Salt", "Silver",
	"South", "Stan", "Stone", "Summer", "Thorn", "Well", "West", "Whit",
	"Willow", "Win", "Winter", "Wolf", "Wood", "York",
}

var angloPlaceSuffix = []string{
	"borough", "bridge", "brook", "bury", "chester", "combe", "dale", "den",
	"field", "ford", "gate", "ham", "haven", "hill", "holme", "hurst",
	"ington", "leigh", "ley", "mere", "minster", "mouth", "pool", "port",
	"shaw", "stead", "stoke", "ton", "vale", "wick", "wood", "worth",
}

var latinPlacePrefix = []string{
	"San", "Santa", "Villa", "Puerto", "Monte", "Río", "Costa", "Valle",
	"Punta", "Alto", "Campo", "Cerro",
}

var latinPlaceRoot = []string{
	"Adelmo", "Aurelio", "Bellavista", "Bravo", "Clemente", "Colinas",
	"Dorado", "Esperanza", "Esteban", "Fierro", "Florido", "Galeano",
	"Herrera", "Ibarra", "Jacinto", "Laurel", "Lozano", "Marino", "Mateo",
	"Mirador", "Navarro", "Olmedo", "Palma", "Pinares", "Quintero",
	"Riachuelo", "Robles", "Rosales", "Salvador", "Serrano", "Teodoro",
	"Urbina", "Valdés", "Zamora",
}

var continentalPlacePrefix = []string{
	"Adler", "Berg", "Birken", "Eichen", "Falken", "Grün", "Hafen", "Hoch",
	"Kaiser", "Königs", "Kron", "Linden", "Nord", "Ost", "Rhein", "Rosen",
	"Schwarz", "Silber", "Sonnen", "Stein", "Süd", "Tannen", "Wald",
	"Weiss", "Winter", "Wolfs",
}

var continentalPlaceSuffix = []string{
	"bach", "berg", "borg", "bruck", "burg", "dorf", "feld", "hafen",
	"hausen", "heim", "hof", "holm", "stadt", "stein", "sund", "tal",
	"vik", "wald",
}

var eastAsianPlaceParts = [3][2][]string{
	{ // Korean-style
		{"Han", "Seo", "Dae", "Gang", "Nam", "Buk", "Cheon", "Su", "Gwang",
			"Jin", "Hae", "Yeong", "Chang", "Po", "Mok", "Won", "An", "Gye"},
		{"cheon", "ju", "san", "po", "won", "yang", "gok", "jin", "hae",
			"seong", "dong", "rim"},
	},
	{ // Japanese-style
		{"Aka", "Aoi", "Fuji", "Hana", "Haru", "Hoshi", "Kawa", "Kita",
			"Kuro", "Matsu", "Mina", "Naka", "Nishi", "Saka", "Shira",
			"Taka", "Toyo", "Yama", "Yuki"},
		{"bashi", "gawa", "hama", "machi", "moto", "mura", "oka", "saki",
			"shima", "yama", "zaki", "zawa"},
	},
	{ // Chinese-style
		{"Bei", "Chang", "Dong", "Feng", "Hai", "Jin", "Long", "Nan",
			"Qing", "Shan", "Tian", "Xi", "Yun", "Zhong"},
		{"an", "chuan", "du", "hai", "jiang", "lin", "ning", "shan", "tan",
			"xing", "yang", "zhou"},
	},
}

func placeName(r *rand.Rand, c Culture) string {
	switch c {
	case CultureAnglo:
		return angloPlacePrefix[r.IntN(len(angloPlacePrefix))] +
			angloPlaceSuffix[r.IntN(len(angloPlaceSuffix))]
	case CultureLatin:
		if r.IntN(3) == 0 {
			return latinPlaceRoot[r.IntN(len(latinPlaceRoot))]
		}
		return latinPlacePrefix[r.IntN(len(latinPlacePrefix))] + " " +
			latinPlaceRoot[r.IntN(len(latinPlaceRoot))]
	case CultureContinental:
		return continentalPlacePrefix[r.IntN(len(continentalPlacePrefix))] +
			continentalPlaceSuffix[r.IntN(len(continentalPlaceSuffix))]
	default: // East Asian
		parts := eastAsianPlaceParts[r.IntN(len(eastAsianPlaceParts))]
		return parts[0][r.IntN(len(parts[0]))] + parts[1][r.IntN(len(parts[1]))]
	}
}

// ---- Person names ----

var angloGiven = []string{
	"Aaron", "Adam", "Alfie", "Archie", "Ben", "Billy", "Callum", "Charlie",
	"Connor", "Curtis", "Danny", "Dean", "Dylan", "Elliot", "Ewan", "Finlay",
	"Freddie", "George", "Glenn", "Harry", "Harvey", "Jack", "Jake", "James",
	"Jamie", "Joe", "Jordan", "Josh", "Kieran", "Lewis", "Liam", "Luke",
	"Mason", "Matty", "Max", "Nathan", "Ollie", "Owen", "Reece", "Rhys",
	"Robbie", "Ronnie", "Ryan", "Sam", "Scott", "Sean", "Stephen", "Theo",
	"Toby", "Tom", "Tyler", "Will",
}

var angloFamily = []string{
	"Adams", "Allen", "Atkinson", "Bailey", "Baker", "Barnes", "Bell",
	"Bennett", "Brooks", "Burton", "Byrne", "Carter", "Clarke", "Cole",
	"Collins", "Cooper", "Cox", "Davies", "Dawson", "Dixon", "Doyle",
	"Ellis", "Evans", "Fletcher", "Foster", "Gallagher", "Gibson", "Graham",
	"Gray", "Green", "Griffiths", "Hall", "Harris", "Harrison", "Hayes",
	"Holmes", "Hughes", "Hunt", "Jenkins", "Johnson", "Jones", "Kelly",
	"Kennedy", "King", "Lawson", "Lloyd", "Marsh", "Mason", "McCarthy",
	"Mills", "Mitchell", "Moore", "Morgan", "Murphy", "Murray", "Nolan",
	"O'Brien", "O'Connor", "Palmer", "Parker", "Pearce", "Phillips",
	"Powell", "Price", "Reid", "Richards", "Roberts", "Robinson", "Rogers",
	"Shaw", "Simpson", "Smith", "Stevens", "Stone", "Sutton", "Taylor",
	"Thomas", "Thompson", "Turner", "Walker", "Walsh", "Ward", "Watson",
	"Webb", "White", "Wilson", "Wood", "Wright", "Young",
}

var latinGiven = []string{
	"Alejandro", "Andrés", "Ángel", "Antonio", "Bruno", "Carlos", "César",
	"Cristian", "Daniel", "Diego", "Eduardo", "Emilio", "Enzo", "Esteban",
	"Facundo", "Felipe", "Fernando", "Francisco", "Gabriel", "Gonzalo",
	"Hugo", "Ignacio", "Iván", "Javier", "Joaquín", "Jorge", "José", "Juan",
	"Julián", "Leandro", "Lucas", "Luis", "Manuel", "Marcelo", "Marcos",
	"Mateo", "Matías", "Miguel", "Nicolás", "Pablo", "Pedro", "Rafael",
	"Ramón", "Raúl", "Ricardo", "Roberto", "Rodrigo", "Santiago",
	"Sebastián", "Sergio", "Thiago", "Tomás", "Vicente", "Víctor",
}

var latinFamily = []string{
	"Acosta", "Aguilar", "Álvarez", "Benítez", "Blanco", "Cabrera",
	"Campos", "Cardoso", "Carrillo", "Castillo", "Castro", "Correa",
	"Cruz", "Delgado", "Díaz", "Domínguez", "Duarte", "Escobar",
	"Espinoza", "Fernández", "Ferreira", "Figueroa", "Flores", "Franco",
	"Fuentes", "García", "Gómez", "González", "Guerrero", "Gutiérrez",
	"Hernández", "Herrera", "Ibáñez", "Jiménez", "López", "Luna",
	"Márquez", "Martínez", "Medina", "Méndez", "Mendoza", "Molina",
	"Morales", "Moreno", "Muñoz", "Navarro", "Núñez", "Ortega", "Ortiz",
	"Paredes", "Peña", "Pereira", "Pérez", "Quiroga", "Ramírez", "Ramos",
	"Reyes", "Ríos", "Rivas", "Rivera", "Rodríguez", "Rojas", "Romero",
	"Ruiz", "Salazar", "Sánchez", "Santos", "Silva", "Sosa", "Soto",
	"Suárez", "Torres", "Valdez", "Vargas", "Vega", "Vera", "Villalba",
	"Zambrano",
}

var continentalGiven = []string{
	"Anders", "Andreas", "Anton", "Arne", "Bastian", "Casper", "Christoph",
	"Daan", "David", "Emil", "Erik", "Felix", "Filip", "Florian", "Fredrik",
	"Hannes", "Henrik", "Jakob", "Jan", "Jens", "Joachim", "Jonas", "Joris",
	"Julian", "Kasper", "Klaas", "Lars", "Lasse", "Leon", "Linus", "Luca",
	"Lukas", "Magnus", "Marcel", "Marius", "Matthias", "Mikkel", "Milan",
	"Moritz", "Niklas", "Nils", "Oskar", "Paul", "Pieter", "Rasmus", "Robin",
	"Ruben", "Sander", "Sebastian", "Simon", "Sven", "Thijs", "Timo",
	"Tobias", "Torben", "Viktor",
}

var continentalFamily = []string{
	"Andersen", "Bauer", "Becker", "Berg", "Bergström", "Brandt", "Braun",
	"Carlsen", "Dahl", "de Boer", "de Jong", "Dijkstra", "Eriksen",
	"Fischer", "Frank", "Hansen", "Hartmann", "Haugen", "Hendriks",
	"Hermann", "Hoffmann", "Holm", "Jansen", "Jensen", "Johansson",
	"Keller", "Koch", "König", "Krause", "Kristensen", "Krüger", "Larsen",
	"Lehmann", "Lindgren", "Lorenz", "Lund", "Madsen", "Meijer", "Meyer",
	"Möller", "Mulder", "Müller", "Neumann", "Nielsen", "Nilsson", "Novak",
	"Olsen", "Pedersen", "Peters", "Richter", "Roth", "Schmidt",
	"Schneider", "Scholz", "Schröder", "Schulz", "Smit", "Sørensen",
	"Stein", "Svensson", "van den Berg", "van Dijk", "Visser", "Vogel",
	"Voss", "Wagner", "Weber", "Wolf", "Zimmermann",
}

// East Asian person sub-pools: {family, given} per sub-style, romanized,
// rendered family-name-first.
var eastAsianNames = [3][2][]string{
	{ // Korean
		{"Kim", "Lee", "Park", "Choi", "Jung", "Kang", "Cho", "Yoon",
			"Jang", "Lim", "Han", "Oh", "Seo", "Shin", "Kwon", "Hwang",
			"Ahn", "Song", "Yoo", "Hong"},
		{"Min-jun", "Seo-jun", "Do-yun", "Ha-jun", "Ji-ho", "Ji-hu",
			"Jun-seo", "Hyun-woo", "Ji-hoon", "Woo-jin", "Sung-min",
			"Jae-hyun", "Seung-hyun", "Tae-yang", "Young-ho", "Dong-hyun",
			"Kyung-min", "Sang-woo", "In-seong", "Chan-woo"},
	},
	{ // Japanese
		{"Sato", "Suzuki", "Takahashi", "Tanaka", "Watanabe", "Ito",
			"Yamamoto", "Nakamura", "Kobayashi", "Kato", "Yoshida",
			"Yamada", "Sasaki", "Matsumoto", "Inoue", "Kimura", "Hayashi",
			"Shimizu", "Mori", "Abe"},
		{"Haruto", "Yuto", "Sota", "Yuki", "Hayato", "Haruki", "Ryusei",
			"Koki", "Sora", "Sosuke", "Riku", "Takumi", "Kaito", "Ren",
			"Hiroto", "Daiki", "Kenta", "Shota", "Yuma", "Itsuki"},
	},
	{ // Chinese
		{"Wang", "Li", "Zhang", "Liu", "Chen", "Yang", "Huang", "Zhao",
			"Wu", "Zhou", "Xu", "Sun", "Ma", "Zhu", "Hu", "Guo", "He",
			"Lin", "Gao", "Luo"},
		{"Wei", "Jun", "Hao", "Ming", "Lei", "Qiang", "Yong", "Jian",
			"Bo", "Chao", "Feng", "Peng", "Tao", "Kai", "Rui", "Zhen",
			"Xin", "Yu", "Cheng", "Liang"},
	},
}

func personName(r *rand.Rand, c Culture) string {
	switch c {
	case CultureAnglo:
		return angloGiven[r.IntN(len(angloGiven))] + " " +
			angloFamily[r.IntN(len(angloFamily))]
	case CultureLatin:
		return latinGiven[r.IntN(len(latinGiven))] + " " +
			latinFamily[r.IntN(len(latinFamily))]
	case CultureContinental:
		return continentalGiven[r.IntN(len(continentalGiven))] + " " +
			continentalFamily[r.IntN(len(continentalFamily))]
	default: // East Asian: family first, sub-pools never cross-mix
		pool := eastAsianNames[r.IntN(len(eastAsianNames))]
		return pool[0][r.IntN(len(pool[0]))] + " " + pool[1][r.IntN(len(pool[1]))]
	}
}

// ---- Club & stadium names ----

var clubPatterns = map[Culture][]string{
	CultureAnglo: {"%s Athletic", "%s United", "%s Town", "%s City",
		"%s Rovers", "%s Wanderers", "%s Albion", "%s County", "AFC %s", "%s FC"},
	CultureLatin: {"Real %s", "Deportivo %s", "Atlético %s", "Club %s",
		"CD %s", "%s FC", "Unión %s", "Sporting %s"},
	CultureContinental: {"FC %s", "SV %s", "SC %s", "1. FC %s", "Union %s",
		"Dynamo %s", "Rot-Weiss %s"},
	CultureEastAsian: {"%s FC", "FC %s", "%s United", "%s SC"},
}

// clubName rolls a pattern; Continental clubs sometimes carry a founding
// year instead ("Steinburg 1904").
func clubName(r *rand.Rand, c Culture, place string) string {
	if c == CultureContinental && r.IntN(4) == 0 {
		return fmt.Sprintf("%s %d", place, 1874+r.IntN(40))
	}
	pats := clubPatterns[c]
	return fmt.Sprintf(pats[r.IntN(len(pats))], place)
}

// shortName derives a 3-letter uppercase tag from the place, sliding the
// window (then appending digits) on collision.
func shortName(nr *nameRegistry, place string) string {
	letters := []rune{}
	for _, ch := range strings.ToUpper(place) {
		if ch >= 'A' && ch <= 'Z' {
			letters = append(letters, ch)
		}
	}
	for len(letters) < 3 {
		letters = append(letters, 'X')
	}
	for i := 0; i+3 <= len(letters); i++ {
		tag := "#" + string(letters[i:i+3])
		if !nr.used[tag] {
			nr.used[tag] = true
			return string(letters[i : i+3])
		}
	}
	for i := 2; ; i++ {
		tag := fmt.Sprintf("#%s%d", string(letters[0:2]), i)
		if !nr.used[tag] {
			nr.used[tag] = true
			return fmt.Sprintf("%s%d", string(letters[0:2]), i)
		}
	}
}

var stadiumPatterns = map[Culture][]string{
	CultureAnglo:       {"%s Park", "%s Road", "%s Lane", "The %s Ground", "%s Field"},
	CultureLatin:       {"Estadio %s", "Estadio Municipal de %s", "La Bombonera de %s"},
	CultureContinental: {"%s Arena", "%s Stadion", "%s-Park"},
	CultureEastAsian:   {"%s Stadium", "%s Arena", "%s Sports Complex"},
}

func stadiumName(r *rand.Rand, c Culture, place string) string {
	pats := stadiumPatterns[c]
	return fmt.Sprintf(pats[r.IntN(len(pats))], place)
}

// worldName generates a display name when the operator leaves it blank.
func worldName(r *rand.Rand, mix CultureMix) string {
	return placeName(r, pickCulture(r, mix)) + " League"
}

// kitPalette is the TUI-safe color vocabulary (docs/09 §4.1); the Console
// maps these names onto its terminal palette.
var kitPalette = []string{
	"red", "claret", "crimson", "orange", "amber", "yellow", "gold",
	"green", "forest", "teal", "cyan", "sky", "blue", "navy", "royal",
	"purple", "violet", "pink", "white", "black", "grey", "brown",
}
