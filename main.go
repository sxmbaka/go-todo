package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/thedevsaddam/renderer"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var rnd *renderer.Render
var db *mgo.Database

const (
	hostName       string = "localhost:27017"
	dbName         string = "go-todo"
	collectionName string = "todos"
	port           string = ":9000"
)

type (
	// todoModel represents the data model for a todo item in the database (MongoDB) collection "todos"
	// The bson field tags are used by the mgo driver to map the fields of the struct to the fields in the MongoDB documents
	todoModel struct {
		ID        bson.ObjectId `bson:"_id, omitempty"`
		Title     string        `bson:"title"`
		Completed bool          `bson:"completed"`
		CreatedAt time.Time     `bson:"created_at"`
	}
	// todo struct represents the data model for a todo item in the API
	// The json field tags are used by the renderer package to serialize the struct to JSON
	todo struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		Completed bool      `json:"completed"`
		CreatedAt time.Time `json:"created_at"`
	}
)

func init() {
	rnd = renderer.New()
	session, err := mgo.Dial(hostName)
	checkErr(err)
	session.SetMode(mgo.Monotonic, true)
	db = session.DB(dbName)
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func todoHandlers() http.Handler {
	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Get("/", fetchTodos)
		r.Post("/", createTodo)
		r.Put("/{id}", updateTodo)
		r.Delete("/{id}", deleteTodo)
	})
	return router
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	err := rnd.Template(w, http.StatusOK, []string{"static/home.tpl"}, nil)
	checkErr(err)
}

func fetchTodos(w http.ResponseWriter, r *http.Request) {
	todos := []todoModel{}
	if err := db.C(collectionName).Find(bson.M{}).All(&todos); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to fetch the Todos.",
			"error":   err,
		})
		return
	}
	todoList := []todo{}

	for _, t := range todos {
		todoList = append(todoList, todo{
			ID:        t.ID.Hex(),
			Title:     t.Title,
			Completed: t.Completed,
			CreatedAt: t.CreatedAt,
		})
	}
	rnd.JSON(w, http.StatusOK, renderer.M{
		"data": todoList,
	})
}

func createTodo(w http.ResponseWriter, r *http.Request) {
	var t todo
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
		return
	}
	if t.Title == "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "Todo had no title!",
		})
		return
	}
	tm := todoModel{
		ID:        bson.NewObjectId(),
		Title:     t.Title,
		Completed: t.Completed,
		CreatedAt: time.Now(),
	}
	if err := db.C(collectionName).Insert(tm); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to save todo.",
		})
		return
	}
	rnd.JSON(w, http.StatusCreated, renderer.M{
		"message": "Todo created successfully!",
		"todo_id": tm.ID.Hex(),
	})
}

func deleteTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "Id is invalid!",
		})
		return
	}

	if err := db.C(collectionName).RemoveId(bson.ObjectIdHex(id)); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to delete todo!",
			"todo_id": id,
		})
		return
	}

	rnd.JSON(w, http.StatusOK, renderer.M{
		"message": "Todo deleted succesfully!",
		"todo_id": id,
	})
}

func updateTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The id is invalid!",
		})
		return
	}

	var t todo
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
		return
	}

	if t.Title == "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The title is empty!",
		})
		return
	}

	if err := db.C(collectionName).Update(
		bson.M{"_id": bson.ObjectIdHex(id)},
		bson.M{
			"title":     t.Title,
			"completed": t.Completed,
		},
	); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to update todo!",
			"error":   err,
		})
		return
	}
}

func main() {
	stopChannel := make(chan os.Signal)
	signal.Notify(stopChannel, os.Interrupt)

	router := chi.NewRouter()
	router.Use(middleware.Logger)

	router.Get("/", homeHandler)
	router.Mount("/todo", todoHandlers())

	server := &http.Server{
		Addr:         port,
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		log.Println("Listening on port :", port)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Listen : %s\n", err)
		}
	}()
	<-stopChannel
	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	server.Shutdown(ctx)
	defer cancel()
	log.Println("Server stopped.")
}
