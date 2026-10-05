package web

import "embed"

//go:embed templates/*.html templates/_components/*.html templates/*/*.html
var TemplateFS embed.FS

//go:embed static/*
var StaticFS embed.FS
