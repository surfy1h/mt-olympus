package users

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"apollo/database"
	"apollo/redis"
)

const (
	cookieName          = "exchange_userCookie"
	uniqueViolationCode = "23505"
	createUserSql       = `insert into users(email, password) values($1, $2) returning id`
	createAccountSql    = `insert into accounts(user_id, asset_id) values($1, $2) returning id`
	getUserSql          = `select id, email, password, created_at from users where email = $1`
)

func SignUp(c *gin.Context) {
	var (
		payload SignupPayload
		userId  string
	)

	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	if len(payload.Email) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Email required"})
		return
	} else if len(payload.Password) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Password required"})
		return
	}
	payload.Password, _ = argon2id.CreateHash(payload.Password, argon2id.DefaultParams)

	db := database.GetDB()

	// Begin user creation transaction
	tx, err := db.BeginTx(
		c,
		&sql.TxOptions{
			Isolation: sql.LevelSerializable,
			ReadOnly:  false,
		},
	)
	if err != nil {
		log.Println("Error beginning transaction:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred"})
		return
	}

	// Create user account
	stmt, err := tx.Prepare(createUserSql)
	if err != nil {
		log.Println("Error preparing transaction:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred"})
		return
	}
	defer stmt.Close()

	err = stmt.QueryRowContext(c, payload.Email, payload.Password).Scan(&userId)
	if e, ok := err.(*pq.Error); ok {
		if e.Code == uniqueViolationCode {
			c.JSON(http.StatusForbidden, gin.H{"error": "A user with that email already exists."})
		} else {
			log.Println("Error executing user transaction:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred"})
		}

		return
	}

	// Create user accounts
	stmt, err = tx.Prepare(createAccountSql)
	if err != nil {
		log.Println("Error preparing account transaction:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred"})
		return
	}
	defer stmt.Close()

	assets, err := redis.GetAssetList(c)
	if err != nil {
		log.Println("Error retrieving asset list:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred"})
		return
	}
	for _, asset := range assets {
		if _, err = stmt.ExecContext(c, userId, asset.Id); err != nil {
			log.Println("Error executing account transaction:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred"})
			return
		}
	}

	if err = tx.Commit(); err != nil {
		log.Println("Error committing transaction:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred"})
	} else {
		c.JSON(http.StatusCreated, gin.H{"user_id": userId})
	}
}

func Login(c *gin.Context) {
	var (
		payload LoginPayload
		user    User
	)

	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(payload.Email) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Email may not be empty"})
		return
	} else if len(payload.Password) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "password may not be empty"})
		return
	}

	db := database.GetDB()
	err := db.QueryRowContext(c, getUserSql, payload.Email).Scan(
		&user.Id,
		&user.Email,
		&user.Password,
		&user.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusForbidden, gin.H{"error": "User not found"})
		} else {
			log.Println("Error:", err)
			c.JSON(http.StatusForbidden, gin.H{"error": "An error occurred"})
		}
		return
	}

	if match, err := argon2id.ComparePasswordAndHash(payload.Password, user.Password); err != nil {
		log.Println("Error comparing pass & hash:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred"})
		return

	} else if !match {
		c.JSON(http.StatusForbidden, gin.H{"error": "Incorrect email or password"})
		return
	}

	key, err := redis.NewUserSession(user.Id)
	if err != nil {
		log.Println("Error creating new user session:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred"})
		return
	}

	// Cookie variables
	cookieValue := key
	cookieMaxAge := int(time.Hour * 24 * 30)
	const (
		cookiePath     = "/"
		cookieDomain   = ""
		cookieSecure   = false
		cookieHttpOnly = false
	)

	c.SetCookie(
		cookieName,
		cookieValue,
		cookieMaxAge,
		cookiePath,
		cookieDomain,
		cookieSecure,
		cookieHttpOnly,
	)
	c.JSON(http.StatusOK, gin.H{"response": "Login successful"})
}
