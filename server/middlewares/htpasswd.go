package middlewares

import (
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/pkg/utils/random"
	"github.com/OpenListTeam/go-cache"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

const (
	htpasswdCookieName   = "openlist_htpasswd_session"
	htpasswdSessionTTL   = time.Hour
	htpasswdFileCheckInt = 5 * time.Second
)

var (
	htpasswdUsers       = make(map[string]string)
	htpasswdFileModTime time.Time
	htpasswdFilePath    string
	htpasswdMu          sync.RWMutex
	htpasswdSessions    = cache.NewMemCache[string]()
)

func LoadHtpasswd(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	users := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			users[parts[0]] = parts[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	htpasswdMu.Lock()
	htpasswdUsers = users
	htpasswdFilePath = filePath
	htpasswdMu.Unlock()

	info, err := os.Stat(filePath)
	if err == nil {
		htpasswdMu.Lock()
		htpasswdFileModTime = info.ModTime()
		htpasswdMu.Unlock()
	}

	log.Infof("htpasswd file loaded: %s, %d users", filePath, len(users))
	return nil
}

func reloadIfModified() {
	htpasswdMu.RLock()
	filePath := htpasswdFilePath
	lastMod := htpasswdFileModTime
	htpasswdMu.RUnlock()

	if filePath == "" {
		return
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return
	}
	if info.ModTime().After(lastMod) {
		if err := LoadHtpasswd(filePath); err != nil {
			log.Errorf("failed to reload htpasswd file: %v", err)
		}
	}
}

func verifyPassword(hashed, password string) bool {
	if strings.HasPrefix(hashed, "$2y$") || strings.HasPrefix(hashed, "$2a$") || strings.HasPrefix(hashed, "$2b$") {
		return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)) == nil
	}
	if strings.HasPrefix(hashed, "$apr1$") {
		return verifyApr1MD5(hashed, password)
	}
	if strings.HasPrefix(hashed, "{SHA}") {
		h := sha1.Sum([]byte(password))
		return hashed[5:] == hex.EncodeToString(h[:])
	}
	h := md5.Sum([]byte(password))
	return hashed == hex.EncodeToString(h[:])
}

func verifyApr1MD5(hashed, password string) bool {
	parts := strings.Split(hashed, "$")
	if len(parts) != 4 {
		return false
	}
	salt := parts[2]
	expected := parts[3]
	return apr1MD5(password, salt) == expected
}

var apr1MD5Itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func apr1MD5(password, salt string) string {
	ctx := md5.New()
	ctx.Write([]byte(password))
	ctx.Write([]byte("$apr1$"))
	ctx.Write([]byte(salt))

	ctx2 := md5.New()
	ctx2.Write([]byte(password))
	ctx2.Write([]byte(salt))
	ctx2.Write([]byte(password))
	final := ctx2.Sum(nil)

	for i := len(password); i > 0; i -= 16 {
		if i > 16 {
			ctx.Write(final[:16])
		} else {
			ctx.Write(final[:i])
		}
	}

	var nullByte [1]byte
	var pwByte [1]byte
	pwByte[0] = password[0]
	for i := len(password); i > 0; i >>= 1 {
		if i&1 != 0 {
			ctx.Write(nullByte[:])
		} else {
			ctx.Write(pwByte[:])
		}
	}
	final = ctx.Sum(nil)

	for i := 0; i < 1000; i++ {
		ctx1 := md5.New()
		if i&1 != 0 {
			ctx1.Write([]byte(password))
		} else {
			ctx1.Write(final[:16])
		}
		if i%3 != 0 {
			ctx1.Write([]byte(salt))
		}
		if i%7 != 0 {
			ctx1.Write([]byte(password))
		}
		if i&1 != 0 {
			ctx1.Write(final[:16])
		} else {
			ctx1.Write([]byte(password))
		}
		final = ctx1.Sum(nil)
	}

	result := make([]byte, 0, 22)
	result = appendApr1MD5(result, final[0], final[6], final[12], 4)
	result = appendApr1MD5(result, final[1], final[7], final[13], 4)
	result = appendApr1MD5(result, final[2], final[8], final[14], 4)
	result = appendApr1MD5(result, final[3], final[9], final[15], 4)
	result = appendApr1MD5(result, final[4], final[10], final[5], 4)
	result = appendApr1MD5(result, 0, 0, final[11], 2)
	return string(result)
}

func appendApr1MD5(buf []byte, a, b, c byte, n int) []byte {
	value := uint32(a)<<16 | uint32(b)<<8 | uint32(c)
	for i := 0; i < n; i++ {
		buf = append(buf, apr1MD5Itoa64[value&0x3f])
		value >>= 6
	}
	return buf
}

func HtpasswdAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		reloadIfModified()

		cookie, err := c.Cookie(htpasswdCookieName)
		if err == nil {
			_, ok := htpasswdSessions.Get(cookie)
			if ok {
				c.Next()
				return
			}
		}

		user, pass, ok := c.Request.BasicAuth()
		if !ok {
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatus(401)
			return
		}

		htpasswdMu.RLock()
		hashed, userExists := htpasswdUsers[user]
		htpasswdMu.RUnlock()

		if !userExists || !verifyPassword(hashed, pass) {
			c.Header("WWW-Authenticate", `Basic realm="Restricted"`)
			c.AbortWithStatus(401)
			return
		}

		token := random.String(64)
		htpasswdSessions.Set(token, user, cache.WithEx[string](htpasswdSessionTTL))
		c.SetCookie(htpasswdCookieName, token, int(htpasswdSessionTTL.Seconds()), "/", "", false, true)
		c.Next()
	}
}
