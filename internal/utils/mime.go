package utils

func GetExtensionFromMime(mimeType string) string {
	switch mimeType {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "audio/ogg":
		return "ogg"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/wav":
		return "wav"
	case "video/mp4":
		return "mp4"
	case "application/pdf":
		return "pdf"
	default:
		return "bin"
	}
}
