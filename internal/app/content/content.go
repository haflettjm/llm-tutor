package content

import "embed"

//go:embed MENTOR.md
var MentorMD []byte

//go:embed souls
var SoulsFS embed.FS

//go:embed lesson-plans
var LessonPlansFS embed.FS
