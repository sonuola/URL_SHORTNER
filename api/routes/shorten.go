package routes

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/sonu/URL_SHORTNER/api/database"
	"github.com/sonu/URL_SHORTNER/api/models"
	"github.com/sonu/URL_SHORTNER/api/utils"
	"github.com/google/uuid"
)

func ShortenURL(c *gin.Context) {
	var body models.Request

	if err := c.ShouldBind(&body) ; err != nil {
		c.JSON(http.StatusBadRequest,gin.H{"error" : "Cannot Parse JSON"})
		return
	}

	r2 := database.CreateClient(1)
	defer r2.Close()

	val,err := r2.Get(database.Ctx, c.ClientIP()).Result()

    if err == redis.Nil {
		_ = r2.Set(database.Ctx, c.ClientIP(),os.Getenv("API_QUOTA"), 30*60*time.Second).Err()
	} else {
		val,_ = r2.Get(database.Ctx, c.ClientIP()).Result()
		valInt,_ := strconv.Atoi(val)

		if valInt <= 0 {
			limit,_ := r2.TTL(database.Ctx, c.ClientIP()).Result()
			c.JSON(http.StatusServiceUnavailable,gin.H {
				"error" : "rate limit exceeded",
				"rate_limit_reset":limit/time.Nanosecond/time.Minute,
			})
		}
	}

	if !govalidator.IsURL(body.URL) {
		c.JSON(http.StatusBadRequest,gin.H{"error":"Invaliad URL"})
		return
	}

	if !utils.IsDifferentDomain(body.URL) {
		c.JSON(http.StatusServiceUnavailable, gin.H {
			"error":"You can't hack this system :)",
		})
		return
	}

	body.URL = utils.EnsureHTTPPrefix(body.URL)

	var id string

	if body.CustomShort == "" {
		id = uuid.New().String()[:6]
	} else {
		id = body.CustomShort
	}

	r := database.CreateClient(0)
	defer r.Close()

	val,_ = r.Get(database.Ctx, id).Result()

	if val != "" {
		c.JSON(http.StatusForbidden,gin.H{
			"error":"URL custom short already Exists",
		})
		return
	}

	if(body.EXPIRY == 0) {
		body.EXPIRY = 24
	}

	err = r.Set(database.Ctx, id, body.URL, body.EXPIRY * 3600 * time.Second).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":"Unable to connect to the redis server",
		})
		return
	}

	resp := models.Response{
		EXPIRY: body.EXPIRY,
		XRateLimitReset: 30,
		XRateRemaining: 10,
		URL: body.URL,
		CustomShort:"",
	}
	r2.Decr(database.Ctx, c.ClientIP())

	val, _ = r2.Get(database.Ctx, c.ClientIP()).Result()
	resp.XRateRemaining, _ = strconv.Atoi(val)

	ttl, _ := r2.TTL(database.Ctx, c.ClientIP()).Result()
	resp.XRateLimitReset = ttl / time.Nanosecond / time.Minute

	resp.CustomShort = os.Getenv("DOMAIN") + "/" + id

	c.JSON(http.StatusOK, resp)
}