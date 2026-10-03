// Package authtest menyediakan helper untuk mensimulasikan gin.Context yang
// sudah divalidasi auth.Service.RequireAuth, KHUSUS dipakai dari *_test.go
// milik package lain yang perlu menguji handler di belakang RequireAuth
// (mis. job.Handler untuk scoping RBAC teknisi, keputusan 2026-10-03) tanpa
// harus membangun JWT sungguhan tiap test.
//
// Sengaja package TERPISAH dari internal/auth (bukan cuma fungsi exported di
// situ) - supaya "cara memalsukan identitas request" tidak bisa ke-import
// tanpa sengaja dari kode produksi manapun (main.go, handler, service).
// Package ini cuma masuk akal diimpor dari file test, dan secara struktural
// main.go/handler/service tidak punya alasan mengimpor package bernama
// "authtest".
package authtest

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/auth"
	"github.com/nathan/cnc-pm-backend/internal/user"
)

// Middleware mengisi gin.Context persis seperti auth.Service.RequireAuth
// setelah token valid berhasil didekode - pasang di router test SEBELUM
// handler yang diuji. Pakai konstanta ContextKeyUserID/ContextKeyRole yang
// sama persis dengan yang dipakai RequireAuth (bukan string literal
// duplikat), supaya tidak diam-diam berbeda kalau internal/auth berubah.
func Middleware(userID uuid.UUID, role user.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(auth.ContextKeyUserID, userID)
		c.Set(auth.ContextKeyRole, role)
		c.Next()
	}
}
