package providers

import "strings"

// zarLanguageISO turns the site's English language names into the ISO 639
// codes the other source publishes, so "Korean" here and "ko" there are
// recognised as one choice when browsing every source at once (spec 1046).
//
// The site's list is its own — 130-odd entries, some of them not languages at
// all ("Sign Languages", "Apache languages") and one in Persian. A name with no
// code here is still offered when this source is browsed alone; it simply joins
// with nothing, which is the honest answer for a name no code describes.
func zarLanguageISO(name string) string {
	return zarLanguageCodes[strings.ToLower(strings.TrimSpace(name))]
}

// The codes follow the other source's own vocabulary where it departs from ISO
// 639-1 — "cmn" for Mandarin, "yue" for Cantonese, "zxx" for none, and its
// private "qbo"/"qbn" for Serbo-Croatian and Flemish — because the point of the
// table is to join with THAT list, not to be a reference.
var zarLanguageCodes = map[string]string{
	"english": "en", "japanese": "ja", "french": "fr", "korean": "ko", "spanish": "es",
	"castilian": "es", "german": "de", "italian": "it", "mandarin": "cmn", "hindi": "hi",
	"chinese": "zh", "russian": "ru", "turkish": "tr", "cantonese": "yue", "arabic": "ar",
	"portuguese": "pt", "telugu": "te", "tamil": "ta", "latin": "la", "swedish": "sv",
	"polish": "pl", "thai": "th", "malayalam": "ml", "dutch": "nl", "danish": "da",
	"norwegian": "no", "kannada": "kn", "hebrew": "he", "hungarian": "hu",
	"indonesian": "id", "none": "zxx", "greek": "el", "romanian": "ro", "persian": "fa",
	"dari": "fa", "czech": "cs", "finnish": "fi", "ukrainian": "uk", "tagalog": "tl",
	"vietnamese": "vi", "punjabi": "pa", "serbian": "sr", "filipino": "fil",
	"yiddish": "yi", "urdu": "ur", "min nan": "nan", "hokkien": "nan", "catalan": "ca",
	"malay": "ms", "malaysian": "ms", "bengali": "bn", "flemish": "qbn", "icelandic": "is",
	"kurdish": "ku", "irish gaelic": "ga", "gaelic": "gd", "georgian": "ka",
	"marathi": "mr", "afrikaans": "af", "serbo-croatian": "qbo", "swahili": "sw",
	"slovak": "sk", "croatian": "hr", "swiss german": "gsw", "albanian": "sq",
	"bulgarian": "bg", "bosnian": "bs", "basque": "eu", "armenian": "hy",
	"estonian": "et", "macedonian": "mk", "lithuanian": "lt", "slovenian": "sl",
	"galician": "gl", "zulu": "zu", "mongolian": "mn", "hawaiian": "haw",
	"latvian": "lv", "welsh": "cy", "gujarati": "gu", "kazakh": "kk", "sanskrit": "sa",
	"nepali": "ne", "uzbek": "uz", "lao": "lo", "azerbaijani": "az", "maltese": "mt",
	"amharic": "am", "esperanto": "eo", "tibetan": "bo", "xhosa": "xh", "wolof": "wo",
	"somali": "so", "yoruba": "yo", "quechua": "qu", "pashtu": "ps", "tajik": "tg",
	"corsican": "co", "sicilian": "scn", "neapolitan": "nap", "occitan": "oc",
	"scots": "sco", "inuktitut": "iu", "navajo": "nv", "chechen": "ce",
	"lingala": "ln", "central khmer": "km", "khmer": "km", "romany": "rom",
	"maori": "mi", "fijian": "fj", "igbo": "ig", "ibo": "ig", "sotho": "st",
	"abkhazian": "ab", "assyrian neo-aramaic": "aii", "saami": "se",
	"rajasthani": "raj", "berber languages": "ber", "cree": "cr", "sioux": "sio",
	"apache languages": "apa", "mapudungun": "arn", "klingon": "tlh",
	"shanghainese": "wuu", "polynesian": "poz", "kurmanji": "kmr",
}
