package art

// Pixel art lifted from assets/caveira.svg, which is drawn on a 10px grid.
// One character per grid pixel; a space is transparent.

// wordmark is the banner's "caveira" lettering, 41 by 7.
var wordmark = []string{
	"                          #              ",
	"                                         ",
	" ####  ###  #   #  ###   ##   # ##   ### ",
	"#         # #   # #   #   #   ##  #     #",
	"#      #### #   # #####   #   #      ####",
	"#     #   #  # #  #       #   #     #   #",
	" ####  ####   #    ####  ###  #      ####",
}

// pixelSkull is the banner's sword-through-skull mark, 17 by 29. Letters
// index pixelPalette.
var pixelSkull = []string{
	"       BBB       ",
	"        d        ",
	"        d        ",
	"        d        ",
	"        d        ",
	"     BBBBBBB     ",
	"       sSs       ",
	"     #######     ",
	"   ###########   ",
	"  #############  ",
	" ############### ",
	" ############### ",
	"#################",
	"###kkk#####kkk###",
	"##kkkkk###kkkkk##",
	"##kkEEk###kEEkk##",
	"###kkk#####kkk###",
	"g######kkk######g",
	" g######k######g ",
	"  g###########g  ",
	"   ##k#k#k#k##   ",
	"   ###########   ",
	"    #k#k#k#k#    ",
	"    ggggggggg    ",
	"     ggggggg     ",
	"       sSs       ",
	"       sSs       ",
	"        S        ",
	"        S        ",
}

var pixelPalette = map[byte][3]uint8{
	'#': {217, 210, 195},
	'B': {184, 134, 63},
	'd': {58, 42, 32},
	's': {138, 149, 168},
	'S': {201, 209, 218},
	'E': {208, 70, 59},
	'k': {22, 22, 27},
	'g': {168, 159, 142},
	'l': {111, 123, 143},
	'L': {74, 83, 100},
}
