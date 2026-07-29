// Package mediaclass classifies a directly-added download (torrent/URL) into a
// media category from its destination folder and file name. Catalog downloads
// don't use this — their category comes from the catalog. The user can override
// the result at add time; this is only the default guess.
package mediaclass

import (
	"path"
	"strings"

	"synodl/server/internal/store"
)

// folder keyword → category. Matched case-insensitively against each path
// segment of the destination.
var folderBucket = map[string]string{
	"movie": store.CategoryMovie, "movies": store.CategoryMovie,
	"film": store.CategoryMovie, "films": store.CategoryMovie,

	"tv": store.CategorySeries, "tv-show": store.CategorySeries, "tv-shows": store.CategorySeries,
	"tvshow": store.CategorySeries, "tvshows": store.CategorySeries,
	"series": store.CategorySeries, "show": store.CategorySeries, "shows": store.CategorySeries,

	"anime": store.CategoryAnime,

	"music-video": store.CategoryMusicVideo, "music-videos": store.CategoryMusicVideo,
	"musicvideo": store.CategoryMusicVideo, "musicvideos": store.CategoryMusicVideo, "mv": store.CategoryMusicVideo,

	"music": store.CategoryMusic, "songs": store.CategoryMusic,
	"audio": store.CategoryMusic, "album": store.CategoryMusic, "albums": store.CategoryMusic,
}

var audioExt = map[string]bool{
	".mp3": true, ".flac": true, ".m4a": true, ".aac": true, ".wav": true,
	".ogg": true, ".opus": true, ".wma": true, ".alac": true, ".aiff": true,
}

// Classify returns one of the six store categories. An audio file is always
// music (a .flac is a .flac wherever it lands); otherwise the destination folder
// decides; otherwise "other".
func Classify(destination, fileName string) string {
	if audioExt[strings.ToLower(path.Ext(fileName))] {
		return store.CategoryMusic
	}
	for _, seg := range strings.Split(destination, "/") {
		if cat, ok := folderBucket[strings.ToLower(strings.TrimSpace(seg))]; ok {
			return cat
		}
	}
	return store.CategoryOther
}
