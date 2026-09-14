package middleware

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/file-convert-api/internal/ports"
)

var _ ports.RateLimiter = (*RedisRateLimiter)(nil)

// RedisRateLimiter implementa ports.RateLimiter utilizando o protocolo RESP do Redis
// com fallback automático em memória caso o Redis esteja indisponível.
type RedisRateLimiter struct {
	addr        string
	mu          sync.Mutex
	inMemoryMap map[string]int
	inMemoryExp map[string]time.Time
	concurrent  map[string]int
}

// NewRedisRateLimiter cria uma nova instância do adaptador RedisRateLimiter.
func NewRedisRateLimiter(addr string) *RedisRateLimiter {
	if addr == "" {
		addr = "redis:6379"
	}
	return &RedisRateLimiter{
		addr:        addr,
		inMemoryMap: make(map[string]int),
		inMemoryExp: make(map[string]time.Time),
		concurrent:  make(map[string]int),
	}
}

// AllowKey verifica se a chave de API respeita a taxa de requisições configurada.
func (r *RedisRateLimiter) AllowKey(ctx context.Context, apiKey string, limitRPS int) (*ports.RateLimitResult, error) {
	currentSec := time.Now().Unix()
	redisKey := fmt.Sprintf("ratelimit:key:%s:%d", apiKey, currentSec)

	return r.checkWindowLimit(ctx, redisKey, limitRPS)
}

// AllowIP verifica se o endereço IP respeita a taxa de requisições de borda.
func (r *RedisRateLimiter) AllowIP(ctx context.Context, ip string, limitRPS int) (*ports.RateLimitResult, error) {
	currentSec := time.Now().Unix()
	redisKey := fmt.Sprintf("ratelimit:ip:%s:%d", ip, currentSec)

	return r.checkWindowLimit(ctx, redisKey, limitRPS)
}

// AcquireConcurrentJob tenta alocar uma vaga na quota de jobs simultâneos da chave.
func (r *RedisRateLimiter) AcquireConcurrentJob(ctx context.Context, apiKey string, maxConcurrent int) (*ports.RateLimitResult, error) {
	redisKey := fmt.Sprintf("ratelimit:concurrent:%s", apiKey)

	count, err := r.redisIncr(redisKey)
	if err != nil {
		// Fallback em memória
		count = r.inMemoryIncr(redisKey, 0)
	}

	if count > maxConcurrent {
		// Rollback do contador
		_, _ = r.redisDecr(redisKey)
		r.inMemoryDecr(redisKey)

		return &ports.RateLimitResult{
			Allowed:           false,
			Limit:             maxConcurrent,
			Remaining:         0,
			ResetDuration:     5 * time.Second,
			RetryAfterSeconds: 5,
			Reason:            "concurrent_jobs_exceeded",
		}, nil
	}

	remaining := maxConcurrent - count
	if remaining < 0 {
		remaining = 0
	}

	return &ports.RateLimitResult{
		Allowed:           true,
		Limit:             maxConcurrent,
		Remaining:         remaining,
		ResetDuration:     0,
		RetryAfterSeconds: 0,
	}, nil
}

// ReleaseConcurrentJob decrementa o contador de jobs simultâneos ativas.
func (r *RedisRateLimiter) ReleaseConcurrentJob(ctx context.Context, apiKey string) error {
	redisKey := fmt.Sprintf("ratelimit:concurrent:%s", apiKey)

	val, err := r.redisDecr(redisKey)
	if err != nil {
		r.inMemoryDecr(redisKey)
	} else if val < 0 {
		_ = r.redisSetZero(redisKey)
	}

	return nil
}

func (r *RedisRateLimiter) checkWindowLimit(ctx context.Context, key string, limitRPS int) (*ports.RateLimitResult, error) {
	if limitRPS <= 0 {
		limitRPS = 10
	}

	count, err := r.redisIncr(key)
	if err != nil {
		count = r.inMemoryIncr(key, 2*time.Second)
	} else if count == 1 {
		_ = r.redisExpire(key, 2)
	}

	if count > limitRPS {
		return &ports.RateLimitResult{
			Allowed:           false,
			Limit:             limitRPS,
			Remaining:         0,
			ResetDuration:     1 * time.Second,
			RetryAfterSeconds: 1,
			Reason:            "rate_limit_exceeded",
		}, nil
	}

	remaining := limitRPS - count
	if remaining < 0 {
		remaining = 0
	}

	return &ports.RateLimitResult{
		Allowed:           true,
		Limit:             limitRPS,
		Remaining:         remaining,
		ResetDuration:     1 * time.Second,
		RetryAfterSeconds: 0,
	}, nil
}

// --- REDIS RESP DRIVER & IN-MEMORY FALLBACK ---

func (r *RedisRateLimiter) redisIncr(key string) (int, error) {
	conn, err := net.DialTimeout("tcp", r.addr, 200*time.Millisecond)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
	cmd := fmt.Sprintf("*2\r\n$4\r\nINCR\r\n$%d\r\n%s\r\n", len(key), key)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return 0, err
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}

	if strings.HasPrefix(line, ":") {
		numStr := strings.TrimSpace(line[1:])
		return strconv.Atoi(numStr)
	}

	return 0, fmt.Errorf("resposta inesperada do Redis: %s", line)
}

func (r *RedisRateLimiter) redisDecr(key string) (int, error) {
	conn, err := net.DialTimeout("tcp", r.addr, 200*time.Millisecond)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
	cmd := fmt.Sprintf("*2\r\n$4\r\nDECR\r\n$%d\r\n%s\r\n", len(key), key)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return 0, err
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}

	if strings.HasPrefix(line, ":") {
		numStr := strings.TrimSpace(line[1:])
		return strconv.Atoi(numStr)
	}

	return 0, fmt.Errorf("resposta inesperada do Redis: %s", line)
}

func (r *RedisRateLimiter) redisExpire(key string, seconds int) error {
	conn, err := net.DialTimeout("tcp", r.addr, 200*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
	secStr := strconv.Itoa(seconds)
	cmd := fmt.Sprintf("*3\r\n$6\r\nEXPIRE\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(secStr), secStr)
	_, err = conn.Write([]byte(cmd))
	return err
}

func (r *RedisRateLimiter) redisSetZero(key string) error {
	conn, err := net.DialTimeout("tcp", r.addr, 200*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
	cmd := fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$1\r\n0\r\n", len(key), key)
	_, err = conn.Write([]byte(cmd))
	return err
}

// Fallback em memória caso o servidor Redis não responda
func (r *RedisRateLimiter) inMemoryIncr(key string, ttl time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if exp, ok := r.inMemoryExp[key]; ok && now.After(exp) {
		delete(r.inMemoryMap, key)
		delete(r.inMemoryExp, key)
	}

	r.inMemoryMap[key]++
	if ttl > 0 {
		r.inMemoryExp[key] = now.Add(ttl)
	}
	return r.inMemoryMap[key]
}

func (r *RedisRateLimiter) inMemoryDecr(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.inMemoryMap[key] > 0 {
		r.inMemoryMap[key]--
	}
}
