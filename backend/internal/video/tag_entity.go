package video

import "regexp"

// Tag 从视频标题或描述中提取的话题。
type Tag struct {
	ID   uint   `gorm:"primaryKey" json:"id"`                               // 话题唯一 ID
	Name string `gorm:"uniqueIndex;type:varchar(100);not null" json:"name"` // 话题名称，不包含开头的 #
}

// VideoTag 视频与话题的关联记录。
type VideoTag struct {
	ID      uint `gorm:"primaryKey"`     // 关联记录 ID
	VideoID uint `gorm:"index;not null"` // 包含该话题的视频 ID
	TagID   uint `gorm:"index;not null"` // 关联的话题 ID
}

var tagRegex = regexp.MustCompile(`#([\p{L}\p{N}_]+)`)

func ExtractTags(text string) []string {
	matches := tagRegex.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var tags []string
	for _, m := range matches {
		tag := m[1]
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}
