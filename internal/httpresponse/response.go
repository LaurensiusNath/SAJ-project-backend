// Package httpresponse membungkus format response standar yang disepakati
// di docs/api-contract.md: { "success": bool, "data"/"error": ... }.
// Dipakai oleh handler di semua modul, supaya format tidak diketik ulang
// (dan berpotensi tidak konsisten) di tiap modul.
package httpresponse

import "github.com/gin-gonic/gin"

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Success(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

func SuccessWithMeta(c *gin.Context, status int, data any, meta any) {
	c.JSON(status, gin.H{"success": true, "data": data, "meta": meta})
}

func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"success": false, "error": errorBody{Code: code, Message: message}})
}
