// Copyright 2020 gorse Project Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"fmt"
	"github.com/araddon/dateparse"
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/scylladb/go-set"
	"github.com/scylladb/go-set/strset"
	"github.com/thoas/go-funk"
	"github.com/zhenghaoz/gorse/base"
	"github.com/zhenghaoz/gorse/config"
	"github.com/zhenghaoz/gorse/storage/cache"
	"github.com/zhenghaoz/gorse/storage/data"
	"go.uber.org/zap"
	"net/http"
	"strconv"
	"time"
)

// RestServer implements a REST-ful API server.
type RestServer struct {
	CacheClient cache.Database
	DataClient  data.Database
	GorseConfig *config.Config
	HttpHost    string
	HttpPort    int
	IsDashboard bool
	WebService  *restful.WebService
}

// StartHttpServer starts the REST-ful API server.
func (s *RestServer) StartHttpServer() {
	// register restful APIs
	s.CreateWebService()
	restful.DefaultContainer.Add(s.WebService)
	// register swagger UI
	specConfig := restfulspec.Config{
		WebServices: restful.RegisteredWebServices(),
		APIPath:     "/apidocs.json",
	}
	restful.DefaultContainer.Add(restfulspec.NewOpenAPIService(specConfig))
	swaggerFile = specConfig.APIPath
	http.HandleFunc(apiDocsPath, handler)
	// register prometheus
	http.Handle("/metrics", promhttp.Handler())

	base.Logger().Info("start http server",
		zap.String("url", fmt.Sprintf("http://%s:%d", s.HttpHost, s.HttpPort)))
	base.Logger().Fatal("failed to start http server",
		zap.Error(http.ListenAndServe(fmt.Sprintf("%s:%d", s.HttpHost, s.HttpPort), nil)))
}

func LogFilter(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	chain.ProcessFilter(req, resp)
	if req.Request.URL.Path != "/api/dashboard/cluster" &&
		req.Request.URL.Path != "/api/dashboard/tasks" {
		base.Logger().Info(fmt.Sprintf("%s %s", req.Request.Method, req.Request.URL),
			zap.Int("status_code", resp.StatusCode()))
	}
}

// CreateWebService creates web service.
func (s *RestServer) CreateWebService() {
	// Create a server
	ws := s.WebService
	ws.Path("/api/").
		Produces(restful.MIME_JSON).
		Filter(LogFilter)

	/* Interactions with data store */

	// Insert a user
	ws.Route(ws.POST("/user").To(s.insertUser).
		Doc("Insert a user.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"user"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Returns(200, "OK", Success{}).
		Reads(data.User{}))
	// Modify a user
	ws.Route(ws.PATCH("/user/{user-id}").To(s.modifyUser).
		Doc("Modify a user.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"user"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Reads(data.UserPatch{}).
		Returns(200, "OK", Success{}))
	// Get a user
	ws.Route(ws.GET("/user/{user-id}").To(s.getUser).
		Doc("Get a user.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"user"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Returns(200, "OK", data.User{}).
		Writes(data.User{}))
	// Insert users
	ws.Route(ws.POST("/users").To(s.insertUsers).
		Doc("Insert users.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"user"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Returns(200, "OK", Success{}).
		Reads([]data.User{}))
	// Get users
	ws.Route(ws.GET("/users").To(s.getUsers).
		Doc("Get users.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"user"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned users").DataType("integer")).
		Param(ws.QueryParameter("cursor", "cursor for next page").DataType("string")).
		Returns(200, "OK", UserIterator{}).
		Writes(UserIterator{}))
	// Delete a user
	ws.Route(ws.DELETE("/user/{user-id}").To(s.deleteUser).
		Doc("Delete a user.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"user"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Returns(200, "OK", Success{}).
		Writes(Success{}))

	// Insert an item
	ws.Route(ws.POST("/item").To(s.insertItem).
		Doc("Insert an item.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"item"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Returns(200, "OK", Success{}).
		Reads(data.Item{}))
	// Modify an item
	ws.Route(ws.PATCH("/item/{item-id}").To(s.modifyItem).
		Doc("Modify an item.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"item"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("item-id", "identifier of the item").DataType("string")).
		Reads(data.ItemPatch{}).
		Returns(200, "OK", Success{}))
	// Get items
	ws.Route(ws.GET("/items").To(s.getItems).
		Doc("Get items.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"item"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("cursor", "cursor for next page").DataType("string")).
		Returns(200, "OK", ItemIterator{}).
		Writes(ItemIterator{}))
	// Get item
	ws.Route(ws.GET("/item/{item-id}").To(s.getItem).
		Doc("Get a item.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"item"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("item-id", "identifier of the item").DataType("string")).
		Returns(200, "OK", data.Item{}).
		Writes(data.Item{}))
	// Insert items
	ws.Route(ws.POST("/items").To(s.insertItems).
		Doc("Insert items.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"item"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Reads([]data.Item{}))
	// Delete item
	ws.Route(ws.DELETE("/item/{item-id}").To(s.deleteItem).
		Doc("Delete a item.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"item"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("item-id", "identified of the item").DataType("string")).
		Returns(200, "OK", Success{}).
		Writes(Success{}))
	// Insert category
	ws.Route(ws.PUT("/item/{item-id}/category/{category}").To(s.insertItemCategory).
		Doc("Insert a category for a item").
		Metadata(restfulspec.KeyOpenAPITags, []string{"item"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("item-id", "identified of the item").DataType("string")).
		Param(ws.PathParameter("category", "category of the item").DataType("string")).
		Returns(200, "OK", Success{}).
		Writes(Success{}))
	// Delete category
	ws.Route(ws.DELETE("/item/{item-id}/category/{category}").To(s.deleteItemCategory).
		Doc("Delete a category from a item").
		Metadata(restfulspec.KeyOpenAPITags, []string{"item"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("item-id", "identified of the item").DataType("string")).
		Param(ws.PathParameter("category", "category of the item").DataType("string")).
		Returns(200, "OK", Success{}).
		Writes(Success{}))

	// Insert feedback
	ws.Route(ws.POST("/feedback").To(s.insertFeedback(false)).
		Doc("Insert multiple feedback. Ignore insertion if feedback exists.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Reads([]data.Feedback{}).
		Returns(200, "OK", Success{}))
	ws.Route(ws.PUT("/feedback").To(s.insertFeedback(true)).
		Doc("Insert multiple feedback. Existed feedback would be overwritten.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Reads([]data.Feedback{}).
		Returns(200, "OK", Success{}))
	// Get feedback
	ws.Route(ws.GET("/feedback").To(s.getFeedback).
		Doc("Get multiple feedback.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("cursor", "cursor for next page").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned feedback").DataType("integer")).
		Returns(200, "OK", FeedbackIterator{}).
		Writes(FeedbackIterator{}))
	ws.Route(ws.GET("/feedback/{user-id}/{item-id}").To(s.getUserItemFeedback).
		Doc("Get feedback between a user and a item.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.PathParameter("item-id", "identifier of the item").DataType("string")).
		Returns(200, "OK", []data.Feedback{}).
		Writes([]data.Feedback{}))
	ws.Route(ws.DELETE("/feedback/{user-id}/{item-id}").To(s.deleteUserItemFeedback).
		Doc("Delete feedback between a user and a item.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.PathParameter("item-id", "identifier of the item").DataType("string")).
		Returns(200, "OK", []data.Feedback{}).
		Writes([]data.Feedback{}))
	ws.Route(ws.GET("/feedback/{feedback-type}").To(s.getTypedFeedback).
		Doc("Get multiple feedback with feedback type.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.PathParameter("feedback-type", "feedback type").DataType("string")).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("cursor", "cursor for next page").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned feedback").DataType("integer")).
		Returns(200, "OK", FeedbackIterator{}).
		Writes(FeedbackIterator{}))
	ws.Route(ws.GET("/feedback/{feedback-type}/{user-id}/{item-id}").To(s.getTypedUserItemFeedback).
		Doc("Get feedback between a user and a item with feedback type.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.PathParameter("feedback-type", "feedback type").DataType("string")).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.PathParameter("item-id", "identifier of the item").DataType("string")).
		Returns(200, "OK", data.Feedback{}).
		Writes(data.Feedback{}))
	ws.Route(ws.DELETE("/feedback/{feedback-type}/{user-id}/{item-id}").To(s.deleteTypedUserItemFeedback).
		Doc("Delete feedback between a user and a item with feedback type.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.PathParameter("feedback-type", "feedback type").DataType("string")).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.PathParameter("item-id", "identifier of the item").DataType("string")).
		Returns(200, "OK", data.Feedback{}).
		Writes(data.Feedback{}))
	// Get feedback by user id
	ws.Route(ws.GET("/user/{user-id}/feedback/{feedback-type}").To(s.getTypedFeedbackByUser).
		Doc("Get feedback by user id with feedback type.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.PathParameter("feedback-type", "feedback type").DataType("string")).
		Returns(200, "OK", []data.Feedback{}).
		Writes([]data.Feedback{}))
	ws.Route(ws.GET("/user/{user-id}/feedback/").To(s.getFeedbackByUser).
		Doc("Get feedback by user id.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Returns(200, "OK", []data.Feedback{}).
		Writes([]data.Feedback{}))
	// Get feedback by item-id
	ws.Route(ws.GET("/item/{item-id}/feedback/{feedback-type}").To(s.getTypedFeedbackByItem).
		Doc("Get feedback by item id with feedback type.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("item-id", "identifier of the item").DataType("string")).
		Param(ws.PathParameter("feedback-type", "feedback type").DataType("string")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))
	ws.Route(ws.GET("/item/{item-id}/feedback/").To(s.getFeedbackByItem).
		Doc("Get feedback by item id.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"feedback"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("item-id", "identifier of the item").DataType("string")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))

	/* Interaction with intermediate result */

	// Get collaborative filtering recommendation by user id
	ws.Route(ws.GET("/intermediate/recommend/{user-id}").To(s.getCollaborative).
		Doc("get the collaborative filtering recommendation for a user").
		Metadata(restfulspec.KeyOpenAPITags, []string{"intermediate"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset of the list").DataType("integer")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))
	ws.Route(ws.GET("/intermediate/recommend/{user-id}/{category}").To(s.getCategorizedCollaborative).
		Doc("get the collaborative filtering recommendation for a user").
		Metadata(restfulspec.KeyOpenAPITags, []string{"intermediate"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.PathParameter("category", "category of items").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset of the list").DataType("integer")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))

	/* Rank recommendation */

	// Get popular items
	ws.Route(ws.GET("/popular").To(s.getPopular).
		Doc("get popular items").
		Metadata(restfulspec.KeyOpenAPITags, []string{"recommendation"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset of the list").DataType("integer")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))
	ws.Route(ws.GET("/popular/{category}").To(s.getCategoryPopular).
		Doc("get popular items in category").
		Metadata(restfulspec.KeyOpenAPITags, []string{"recommendation"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("category", "category of items").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset of the list").DataType("integer")).
		Returns(http.StatusOK, "OK", []string{}).
		Writes([]string{}))
	// Get latest items
	ws.Route(ws.GET("/latest").To(s.getLatest).
		Doc("get latest items").
		Metadata(restfulspec.KeyOpenAPITags, []string{"recommendation"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset of the list").DataType("integer")).
		Returns(200, "OK", []cache.Scored{}).
		Writes([]cache.Scored{}))
	ws.Route(ws.GET("/latest/{category}").To(s.getCategoryLatest).
		Doc("get latest items in category").
		Metadata(restfulspec.KeyOpenAPITags, []string{"recommendation"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("category", "category of items").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset of the list").DataType("integer")).
		Returns(http.StatusOK, "OK", []string{}).
		Writes([]string{}))
	// Get neighbors
	ws.Route(ws.GET("/item/{item-id}/neighbors/").To(s.getItemNeighbors).
		Doc("get neighbors of a item").
		Metadata(restfulspec.KeyOpenAPITags, []string{"recommendation"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("item-id", "identifier of the item").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset of the list").DataType("integer")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))
	ws.Route(ws.GET("/item/{item-id}/neighbors/{category}").To(s.getItemCategorizedNeighbors).
		Doc("get neighbors of a item").
		Metadata(restfulspec.KeyOpenAPITags, []string{"recommendation"}).
		Param(ws.PathParameter("item-id", "identifier of the item").DataType("string")).
		Param(ws.PathParameter("category", "category of items").DataType("string")).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset of the list").DataType("integer")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))
	ws.Route(ws.GET("/user/{user-id}/neighbors/").To(s.getUserNeighbors).
		Doc("get neighbors of a user").
		Metadata(restfulspec.KeyOpenAPITags, []string{"recommendation"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned users").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset of the list").DataType("integer")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))
	ws.Route(ws.GET("/recommend/{user-id}").To(s.getRecommend).
		Doc("Get recommendation for user.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"recommendation"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.QueryParameter("write-back-type", "type of write back feedback").DataType("string")).
		Param(ws.QueryParameter("write-back-delay", "timestamp delay of write back feedback in minutes").DataType("integer")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset in the recommendation result").DataType("integer")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))
	ws.Route(ws.GET("/recommend/{user-id}/{category}").To(s.getRecommend).
		Doc("Get recommendation for user.").
		Metadata(restfulspec.KeyOpenAPITags, []string{"recommendation"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.PathParameter("user-id", "identifier of the user").DataType("string")).
		Param(ws.PathParameter("category", "category of items").DataType("string")).
		Param(ws.QueryParameter("write-back-type", "type of write back feedback").DataType("string")).
		Param(ws.QueryParameter("write-back-delay", "timestamp delay of write back feedback in minutes").DataType("integer")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Param(ws.QueryParameter("offset", "offset in the recommendation result").DataType("integer")).
		Returns(200, "OK", []string{}).
		Writes([]string{}))

	/* Interaction with measurements */

	ws.Route(ws.GET("/measurements/{name}").To(s.getMeasurements).
		Doc("Get measurements").
		Metadata(restfulspec.KeyOpenAPITags, []string{"measurements"}).
		Param(ws.HeaderParameter("X-API-Key", "secret key for RESTful API").DataType("string")).
		Param(ws.QueryParameter("n", "number of returned items").DataType("integer")).
		Returns(200, "OK", []data.Measurement{}).
		Writes([]data.Measurement{}))
}

// ParseInt parses integers from the query parameter.
func ParseInt(request *restful.Request, name string, fallback int) (value int, err error) {
	valueString := request.QueryParameter(name)
	value, err = strconv.Atoi(valueString)
	if err != nil && valueString == "" {
		value = fallback
		err = nil
	}
	return
}

func (s *RestServer) getList(prefix, name string, request *restful.Request, response *restful.Response) {
	var n, begin, end int
	var err error
	// read arguments
	if begin, err = ParseInt(request, "offset", 0); err != nil {
		BadRequest(response, err)
		return
	}
	if n, err = ParseInt(request, "n", s.GorseConfig.Server.DefaultN); err != nil {
		BadRequest(response, err)
		return
	}
	end = begin + n - 1
	// Get the popular list
	items, err := s.CacheClient.GetScores(prefix, name, begin, end)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	// Send result
	Ok(response, items)
}

// getPopular gets popular items from database.
func (s *RestServer) getPopular(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	base.Logger().Debug("get popular items")
	s.getList(cache.PopularItems, "", request, response)
}

func (s *RestServer) getLatest(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	base.Logger().Debug("get latest items")
	s.getList(cache.LatestItems, "", request, response)
}

func (s *RestServer) getCategoryPopular(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	category := request.PathParameter("category")
	base.Logger().Debug("get category popular items in category", zap.String("category", category))
	s.getList(cache.PopularItems, category, request, response)
}

func (s *RestServer) getCategoryLatest(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	category := request.PathParameter("category")
	base.Logger().Debug("get category latest items in category", zap.String("category", category))
	s.getList(cache.LatestItems, category, request, response)
}

// get feedback by item-id with feedback type
func (s *RestServer) getTypedFeedbackByItem(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	feedbackType := request.PathParameter("feedback-type")
	itemId := request.PathParameter("item-id")
	feedback, err := s.DataClient.GetItemFeedback(itemId, feedbackType)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, feedback)
}

// get feedback by item-id
func (s *RestServer) getFeedbackByItem(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	itemId := request.PathParameter("item-id")
	feedback, err := s.DataClient.GetItemFeedback(itemId)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, feedback)
}

// getItemNeighbors gets neighbors of a item from database.
func (s *RestServer) getItemNeighbors(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Get item id
	itemId := request.PathParameter("item-id")
	s.getList(cache.ItemNeighbors, itemId, request, response)
}

// getItemCategorizedNeighbors gets categorized neighbors of an item from database.
func (s *RestServer) getItemCategorizedNeighbors(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Get item id
	itemId := request.PathParameter("item-id")
	category := request.PathParameter("category")
	s.getList(cache.ItemNeighbors, itemId+"/"+category, request, response)
}

// getUserNeighbors gets neighbors of a user from database.
func (s *RestServer) getUserNeighbors(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Get item id
	userId := request.PathParameter("user-id")
	s.getList(cache.UserNeighbors, userId, request, response)
}

// getSubscribe gets subscribed items of a user from database.
//func (s *RestServer) getSubscribe(request *restful.Request, response *restful.Response) {
//	// Authorize
//	if !s.auth(request, response) {
//		return
//	}
//	// Get user id
//	userId := request.PathParameter("user-id")
//	s.getList(cache.SubscribeItems, userId, request, response)
//}

// getCategorizedCollaborative gets cached categorized recommended items from database.
func (s *RestServer) getCategorizedCollaborative(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Get user id
	userId := request.PathParameter("user-id")
	category := request.PathParameter("category")
	s.getList(cache.OfflineRecommend, userId+"/"+category, request, response)
}

// getCollaborative gets cached recommended items from database.
func (s *RestServer) getCollaborative(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Get user id
	userId := request.PathParameter("user-id")
	s.getList(cache.OfflineRecommend, userId, request, response)
}

// Recommend items to users.
// 1. If there are recommendations in cache, return cached recommendations.
// 2. If there are historical interactions of the users, return similar items.
// 3. Otherwise, return fallback recommendation (popular/latest).
func (s *RestServer) Recommend(userId, category string, n int, recommenders ...Recommender) ([]string, error) {
	initStart := time.Now()

	// create context
	ctx, err := s.createRecommendContext(userId, category, n)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// execute recommenders
	for _, recommender := range recommenders {
		err = recommender(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// return recommendations
	if len(ctx.results) > n {
		ctx.results = ctx.results[:n]
	}
	totalTime := time.Since(initStart)
	base.Logger().Info("complete recommendation",
		zap.Int("num_from_final", ctx.numFromOffline),
		zap.Int("num_from_collaborative", ctx.numFromCollaborative),
		zap.Int("num_from_item_based", ctx.numFromItemBased),
		zap.Int("num_from_user_based", ctx.numFromUserBased),
		zap.Int("num_from_latest", ctx.numFromLatest),
		zap.Int("num_from_poplar", ctx.numFromPopular),
		zap.Duration("total_time", totalTime),
		zap.Duration("load_final_recommend_time", ctx.loadOfflineRecTime),
		zap.Duration("load_col_recommend_time", ctx.loadColRecTime),
		zap.Duration("load_hist_time", ctx.loadLoadHistTime),
		zap.Duration("item_based_recommend_time", ctx.itemBasedTime),
		zap.Duration("user_based_recommend_time", ctx.userBasedTime),
		zap.Duration("load_latest_time", ctx.loadLatestTime),
		zap.Duration("load_popular_time", ctx.loadPopularTime))
	return ctx.results, nil
}

type recommendContext struct {
	userId       string
	category     string
	userFeedback []data.Feedback
	n            int
	results      []string
	excludeSet   *strset.Set

	numPrevStage         int
	numFromLatest        int
	numFromPopular       int
	numFromUserBased     int
	numFromItemBased     int
	numFromCollaborative int
	numFromOffline       int

	loadOfflineRecTime time.Duration
	loadColRecTime     time.Duration
	loadLoadHistTime   time.Duration
	itemBasedTime      time.Duration
	userBasedTime      time.Duration
	loadLatestTime     time.Duration
	loadPopularTime    time.Duration
}

func (s *RestServer) createRecommendContext(userId, category string, n int) (*recommendContext, error) {
	// pull ignored items
	ignoreItems, err := s.CacheClient.GetScores(cache.IgnoreItems, userId, 0, -1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	excludeSet := strset.New()
	for _, item := range ignoreItems {
		if item.Score <= float32(time.Now().Unix()) {
			excludeSet.Add(item.Id)
		}
	}
	return &recommendContext{
		userId:     userId,
		category:   category,
		n:          n,
		excludeSet: excludeSet,
	}, nil
}

func (s *RestServer) requireUserFeedback(ctx *recommendContext) error {
	if ctx.userFeedback == nil {
		start := time.Now()
		var err error
		ctx.userFeedback, err = s.DataClient.GetUserFeedback(ctx.userId, false)
		if err != nil {
			return errors.Trace(err)
		}
		for _, feedback := range ctx.userFeedback {
			ctx.excludeSet.Add(feedback.ItemId)
		}
		ctx.loadLoadHistTime = time.Since(start)
	}
	return nil
}

func (s *RestServer) filterOutHiddenScores(items []cache.Scored) []cache.Scored {
	isHidden, err := s.CacheClient.Exists(cache.HiddenItems, cache.RemoveScores(items)...)
	if err != nil {
		base.Logger().Error("failed to check hidden items", zap.Error(err))
		return items
	}
	var results []cache.Scored
	for i := range isHidden {
		if isHidden[i] == 0 {
			results = append(results, items[i])
		}
	}
	return results
}

func (s *RestServer) filterOutHiddenFeedback(feedbacks []data.Feedback) []data.Feedback {
	names := make([]string, len(feedbacks))
	for i, item := range feedbacks {
		names[i] = item.ItemId
	}
	isHidden, err := s.CacheClient.Exists(cache.HiddenItems, names...)
	if err != nil {
		base.Logger().Error("failed to check hidden items", zap.Error(err))
		return feedbacks
	}
	var results []data.Feedback
	for i := range isHidden {
		if isHidden[i] == 0 {
			results = append(results, feedbacks[i])
		}
	}
	return results
}

type Recommender func(ctx *recommendContext) error

func (s *RestServer) RecommendOffline(ctx *recommendContext) error {
	if len(ctx.results) < ctx.n {
		start := time.Now()
		recommendation, err := s.CacheClient.GetCategoryScores(cache.OfflineRecommend, ctx.userId, ctx.category, 0, s.GorseConfig.Database.CacheSize)
		if err != nil {
			return errors.Trace(err)
		}
		recommendation = s.filterOutHiddenScores(recommendation)
		for _, item := range recommendation {
			if !ctx.excludeSet.Has(item.Id) {
				ctx.results = append(ctx.results, item.Id)
				ctx.excludeSet.Add(item.Id)
			}
		}
		ctx.loadOfflineRecTime = time.Since(start)
		LoadCTRRecommendCacheSeconds.Observe(ctx.loadOfflineRecTime.Seconds())
		ctx.numFromOffline = len(ctx.results) - ctx.numPrevStage
		ctx.numPrevStage = len(ctx.results)
	}
	return nil
}

func (s *RestServer) RecommendCollaborative(ctx *recommendContext) error {
	if len(ctx.results) < ctx.n {
		start := time.Now()
		collaborativeRecommendation, err := s.CacheClient.GetCategoryScores(cache.CollaborativeRecommend, ctx.userId, ctx.category, 0, s.GorseConfig.Database.CacheSize)
		if err != nil {
			return errors.Trace(err)
		}
		collaborativeRecommendation = s.filterOutHiddenScores(collaborativeRecommendation)
		for _, item := range collaborativeRecommendation {
			if !ctx.excludeSet.Has(item.Id) {
				ctx.results = append(ctx.results, item.Id)
				ctx.excludeSet.Add(item.Id)
			}
		}
		ctx.loadColRecTime = time.Since(start)
		LoadCollaborativeRecommendCacheSeconds.Observe(ctx.loadColRecTime.Seconds())
		ctx.numFromCollaborative = len(ctx.results) - ctx.numPrevStage
		ctx.numPrevStage = len(ctx.results)
	}
	return nil
}

func (s *RestServer) RecommendUserBased(ctx *recommendContext) error {
	if len(ctx.results) < ctx.n {
		err := s.requireUserFeedback(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		start := time.Now()
		candidates := make(map[string]float32)
		// load similar users
		similarUsers, err := s.CacheClient.GetScores(cache.UserNeighbors, ctx.userId, 0, s.GorseConfig.Database.CacheSize)
		if err != nil {
			return errors.Trace(err)
		}
		for _, user := range similarUsers {
			// load historical feedback
			feedbacks, err := s.DataClient.GetUserFeedback(user.Id, false, s.GorseConfig.Database.PositiveFeedbackType...)
			if err != nil {
				return errors.Trace(err)
			}
			feedbacks = s.filterOutHiddenFeedback(feedbacks)
			// add unseen items
			for _, feedback := range feedbacks {
				if !ctx.excludeSet.Has(feedback.ItemId) {
					item, err := s.DataClient.GetItem(feedback.ItemId)
					if err != nil {
						return errors.Trace(err)
					}
					if ctx.category == "" || funk.ContainsString(item.Categories, ctx.category) {
						candidates[feedback.ItemId] += user.Score
					}
				}
			}
		}
		// collect top k
		k := ctx.n - len(ctx.results)
		filter := base.NewTopKStringFilter(k)
		for id, score := range candidates {
			filter.Push(id, score)
		}
		ids, _ := filter.PopAll()
		ctx.results = append(ctx.results, ids...)
		ctx.excludeSet.Add(ids...)
		ctx.userBasedTime = time.Since(start)
		UserBasedRecommendSeconds.Observe(ctx.userBasedTime.Seconds())
		ctx.numFromUserBased = len(ctx.results) - ctx.numPrevStage
		ctx.numPrevStage = len(ctx.results)
	}
	return nil
}

func (s *RestServer) RecommendItemBased(ctx *recommendContext) error {
	if len(ctx.results) < ctx.n {
		err := s.requireUserFeedback(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		start := time.Now()
		// collect candidates
		candidates := make(map[string]float32)
		for _, feedback := range ctx.userFeedback {
			// load similar items
			similarItems, err := s.CacheClient.GetCategoryScores(cache.ItemNeighbors, feedback.ItemId, ctx.category, 0, s.GorseConfig.Database.CacheSize)
			if err != nil {
				return errors.Trace(err)
			}
			// add unseen items
			similarItems = s.filterOutHiddenScores(similarItems)
			for _, item := range similarItems {
				if !ctx.excludeSet.Has(item.Id) {
					candidates[item.Id] += item.Score
				}
			}
		}
		// collect top k
		k := ctx.n - len(ctx.results)
		filter := base.NewTopKStringFilter(k)
		for id, score := range candidates {
			filter.Push(id, score)
		}
		ids, _ := filter.PopAll()
		ctx.results = append(ctx.results, ids...)
		ctx.excludeSet.Add(ids...)
		ctx.itemBasedTime = time.Since(start)
		ItemBasedRecommendSeconds.Observe(ctx.itemBasedTime.Seconds())
		ctx.numFromItemBased = len(ctx.results) - ctx.numPrevStage
		ctx.numPrevStage = len(ctx.results)
	}
	return nil
}

func (s *RestServer) RecommendLatest(ctx *recommendContext) error {
	if len(ctx.results) < ctx.n {
		err := s.requireUserFeedback(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		start := time.Now()
		items, err := s.CacheClient.GetScores(cache.LatestItems, ctx.category, 0, ctx.n-len(ctx.results))
		if err != nil {
			return errors.Trace(err)
		}
		items = s.filterOutHiddenScores(items)
		for _, item := range items {
			if !ctx.excludeSet.Has(item.Id) {
				ctx.results = append(ctx.results, item.Id)
				ctx.excludeSet.Add(item.Id)
			}
		}
		ctx.loadLatestTime = time.Since(start)
		LoadLatestRecommendCacheSeconds.Observe(ctx.loadLatestTime.Seconds())
		ctx.numFromLatest = len(ctx.results) - ctx.numPrevStage
		ctx.numPrevStage = len(ctx.results)
	}
	return nil
}

func (s *RestServer) RecommendPopular(ctx *recommendContext) error {
	if len(ctx.results) < ctx.n {
		err := s.requireUserFeedback(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		start := time.Now()
		items, err := s.CacheClient.GetScores(cache.PopularItems, ctx.category, 0, ctx.n-len(ctx.results))
		if err != nil {
			return errors.Trace(err)
		}
		items = s.filterOutHiddenScores(items)
		for _, item := range items {
			if !ctx.excludeSet.Has(item.Id) {
				ctx.results = append(ctx.results, item.Id)
				ctx.excludeSet.Add(item.Id)
			}
		}
		ctx.loadPopularTime = time.Since(start)
		LoadPopularRecommendCacheSeconds.Observe(ctx.loadPopularTime.Seconds())
		ctx.numFromPopular = len(ctx.results) - ctx.numPrevStage
		ctx.numPrevStage = len(ctx.results)
	}
	return nil
}

func (s *RestServer) getRecommend(request *restful.Request, response *restful.Response) {
	startTime := time.Now()
	// authorize
	if !s.auth(request, response) {
		return
	}
	// parse arguments
	userId := request.PathParameter("user-id")
	n, err := ParseInt(request, "n", s.GorseConfig.Server.DefaultN)
	if err != nil {
		BadRequest(response, err)
		return
	}
	category := request.PathParameter("category")
	offset, err := ParseInt(request, "offset", 0)
	if err != nil {
		BadRequest(response, err)
		return
	}
	writeBackFeedback := request.QueryParameter("write-back-type")
	writeBackDelay, err := ParseInt(request, "write-back-delay", 0)
	if err != nil {
		BadRequest(response, err)
		return
	}
	// online recommendation
	recommenders := []Recommender{s.RecommendOffline}
	for _, recommender := range s.GorseConfig.Recommend.FallbackRecommend {
		switch recommender {
		case "collaborative":
			recommenders = append(recommenders, s.RecommendCollaborative)
		case "item_based":
			recommenders = append(recommenders, s.RecommendItemBased)
		case "user_based":
			recommenders = append(recommenders, s.RecommendUserBased)
		case "latest":
			recommenders = append(recommenders, s.RecommendLatest)
		case "popular":
			recommenders = append(recommenders, s.RecommendPopular)
		default:
			InternalServerError(response, fmt.Errorf("unknown fallback recommendation method `%s`", recommender))
			return
		}
	}
	results, err := s.Recommend(userId, category, offset+n, recommenders...)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	results = results[offset:]
	// write back
	if writeBackFeedback != "" {
		for _, itemId := range results {
			// insert to data store
			feedback := data.Feedback{
				FeedbackKey: data.FeedbackKey{
					UserId:       userId,
					ItemId:       itemId,
					FeedbackType: writeBackFeedback,
				},
				Timestamp: time.Now().Add(time.Minute * time.Duration(writeBackDelay)),
			}
			err = s.DataClient.BatchInsertFeedback([]data.Feedback{feedback}, false, false, false)
			if err != nil {
				InternalServerError(response, err)
				return
			}
			// insert to cache store
			err = s.InsertFeedbackToCache([]data.Feedback{feedback})
			if err != nil {
				InternalServerError(response, err)
				return
			}
		}
	}
	GetRecommendSeconds.Observe(time.Since(startTime).Seconds())
	// Send result
	Ok(response, results)
}

// Success is the returned data structure for data insert operations.
type Success struct {
	RowAffected int
}

func (s *RestServer) insertUser(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	temp := data.User{}
	// get userInfo from request and put into temp
	if err := request.ReadEntity(&temp); err != nil {
		BadRequest(response, err)
		return
	}
	if err := s.DataClient.BatchInsertUsers([]data.User{temp}); err != nil {
		InternalServerError(response, err)
		return
	}
	// insert modify timestamp
	if err := s.CacheClient.SetTime(cache.LastModifyUserTime, temp.UserId, time.Now()); err != nil {
		return
	}
	Ok(response, Success{RowAffected: 1})
}

func (s *RestServer) modifyUser(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// get user id
	userId := request.PathParameter("user-id")
	// modify user
	var patch data.UserPatch
	if err := request.ReadEntity(&patch); err != nil {
		BadRequest(response, err)
		return
	}
	if err := s.DataClient.ModifyUser(userId, patch); err != nil {
		InternalServerError(response, err)
		return
	}
	// insert modify timestamp
	if err := s.CacheClient.SetTime(cache.LastModifyUserTime, userId, time.Now()); err != nil {
		return
	}
	Ok(response, Success{RowAffected: 1})
}

func (s *RestServer) getUser(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// get user id
	userId := request.PathParameter("user-id")
	// get user
	user, err := s.DataClient.GetUser(userId)
	if err != nil {
		if errors.IsNotFound(err) {
			PageNotFound(response, err)
		} else {
			InternalServerError(response, err)
		}
		return
	}
	Ok(response, user)
}

func (s *RestServer) insertUsers(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	temp := new([]data.User)
	// get param from request and put into temp
	if err := request.ReadEntity(temp); err != nil {
		BadRequest(response, err)
		return
	}
	var count int
	// range temp and achieve user
	if err := s.DataClient.BatchInsertUsers(*temp); err != nil {
		InternalServerError(response, err)
		return
	}
	for _, user := range *temp {
		// insert modify timestamp
		if err := s.CacheClient.SetTime(cache.LastModifyUserTime, user.UserId, time.Now()); err != nil {
			return
		}
		count++
	}
	Ok(response, Success{RowAffected: count})
}

type UserIterator struct {
	Cursor string
	Users  []data.User
}

func (s *RestServer) getUsers(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	cursor := request.QueryParameter("cursor")
	n, err := ParseInt(request, "n", s.GorseConfig.Server.DefaultN)
	if err != nil {
		BadRequest(response, err)
		return
	}
	// get all users
	cursor, users, err := s.DataClient.GetUsers(cursor, n)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, UserIterator{Cursor: cursor, Users: users})
}

// delete a user by user-id
func (s *RestServer) deleteUser(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// get user-id and put into temp
	userId := request.PathParameter("user-id")
	if err := s.DataClient.DeleteUser(userId); err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, Success{RowAffected: 1})
}

// get feedback by user-id with feedback type
func (s *RestServer) getTypedFeedbackByUser(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	feedbackType := request.PathParameter("feedback-type")
	userId := request.PathParameter("user-id")
	feedback, err := s.DataClient.GetUserFeedback(userId, false, feedbackType)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, feedback)
}

// get feedback by user-id
func (s *RestServer) getFeedbackByUser(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	userId := request.PathParameter("user-id")
	feedback, err := s.DataClient.GetUserFeedback(userId, false)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, feedback)
}

// Item is the data structure for the item but stores the timestamp using string.
type Item struct {
	ItemId     string
	IsHidden   bool
	Categories []string
	Timestamp  string
	Labels     []string
	Comment    string
}

func (s *RestServer) insertItems(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Add ratings
	temp := make([]Item, 0)
	if err := request.ReadEntity(&temp); err != nil {
		BadRequest(response, err)
		return
	}
	// Insert temp
	var count int
	var items []data.Item
	for _, item := range temp {
		// parse datetime
		timestamp, err := dateparse.ParseAny(item.Timestamp)
		if err != nil {
			BadRequest(response, err)
			return
		}
		items = append(items, data.Item{
			ItemId:     item.ItemId,
			IsHidden:   item.IsHidden,
			Categories: item.Categories,
			Timestamp:  timestamp,
			Labels:     item.Labels,
			Comment:    item.Comment,
		})
		count++
	}
	err := s.DataClient.BatchInsertItems(items)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	// insert modify timestamp
	for _, item := range items {
		if err = s.CacheClient.SetTime(cache.LastModifyItemTime, item.ItemId, time.Now()); err != nil {
			return
		}
	}
	Ok(response, Success{RowAffected: count})
}

func (s *RestServer) insertItem(request *restful.Request, response *restful.Response) {
	// authorize
	if !s.auth(request, response) {
		return
	}
	item := new(Item)
	var err error
	if err = request.ReadEntity(item); err != nil {
		BadRequest(response, err)
		return
	}
	// parse datetime
	timestamp, err := dateparse.ParseAny(item.Timestamp)
	if err != nil {
		BadRequest(response, err)
		return
	}
	if err = s.DataClient.BatchInsertItems([]data.Item{{
		ItemId:     item.ItemId,
		IsHidden:   item.IsHidden,
		Categories: item.Categories,
		Timestamp:  timestamp,
		Labels:     item.Labels,
		Comment:    item.Comment,
	}}); err != nil {
		InternalServerError(response, err)
		return
	}
	// insert modify timestamp
	if err = s.CacheClient.SetTime(cache.LastModifyItemTime, item.ItemId, time.Now()); err != nil {
		return
	}
	Ok(response, Success{RowAffected: 1})
}

func (s *RestServer) modifyItem(request *restful.Request, response *restful.Response) {
	// authorize
	if !s.auth(request, response) {
		return
	}
	// Get item id
	itemId := request.PathParameter("item-id")
	// modify item
	var patch data.ItemPatch
	if err := request.ReadEntity(&patch); err != nil {
		BadRequest(response, err)
		return
	}
	if err := s.DataClient.ModifyItem(itemId, patch); err != nil {
		InternalServerError(response, err)
		return
	}
	// insert hidden items to cache
	if patch.IsHidden != nil {
		var err error
		if *patch.IsHidden {
			err = s.CacheClient.SetInt(cache.HiddenItems, itemId, 1)
		} else {
			err = s.CacheClient.Delete(cache.HiddenItems, itemId)
		}
		if err != nil && !errors.IsNotFound(err) {
			InternalServerError(response, err)
			return
		}
	}
	// insert modify timestamp
	if err := s.CacheClient.SetTime(cache.LastModifyItemTime, itemId, time.Now()); err != nil {
		return
	}
	Ok(response, Success{RowAffected: 1})
}

// ItemIterator is the iterator for items.
type ItemIterator struct {
	Cursor string
	Items  []data.Item
}

func (s *RestServer) getItems(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	cursor := request.QueryParameter("cursor")
	n, err := ParseInt(request, "n", s.GorseConfig.Server.DefaultN)
	if err != nil {
		BadRequest(response, err)
		return
	}
	cursor, items, err := s.DataClient.GetItems(cursor, n, nil)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, ItemIterator{Cursor: cursor, Items: items})
}

func (s *RestServer) getItem(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Get item id
	itemId := request.PathParameter("item-id")
	// Get item
	item, err := s.DataClient.GetItem(itemId)
	if err != nil {
		if errors.IsNotFound(err) {
			PageNotFound(response, err)
		} else {
			InternalServerError(response, err)
		}
		return
	}
	Ok(response, item)
}

func (s *RestServer) deleteItem(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	itemId := request.PathParameter("item-id")
	if err := s.DataClient.DeleteItem(itemId); err != nil {
		InternalServerError(response, err)
		return
	}
	// insert deleted item to cache
	if err := s.CacheClient.SetInt(cache.HiddenItems, itemId, 1); err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, Success{RowAffected: 1})
}

func (s *RestServer) insertItemCategory(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Get item id and category
	itemId := request.PathParameter("item-id")
	category := request.PathParameter("category")
	// Insert category
	item, err := s.DataClient.GetItem(itemId)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	if !funk.ContainsString(item.Categories, category) {
		item.Categories = append(item.Categories, category)
	}
	err = s.DataClient.BatchInsertItems([]data.Item{item})
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, Success{RowAffected: 1})
}

func (s *RestServer) deleteItemCategory(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Get item id and category
	itemId := request.PathParameter("item-id")
	category := request.PathParameter("category")
	// Delete category
	item, err := s.DataClient.GetItem(itemId)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	categories := make([]string, 0, len(item.Categories))
	for _, cat := range item.Categories {
		if cat != category {
			categories = append(categories, cat)
		}
	}
	item.Categories = categories
	err = s.DataClient.BatchInsertItems([]data.Item{item})
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, Success{RowAffected: 1})
}

// Feedback is the data structure for the feedback but stores the timestamp using string.
type Feedback struct {
	data.FeedbackKey
	Timestamp string
	Comment   string
}

func (s *RestServer) insertFeedback(overwrite bool) func(request *restful.Request, response *restful.Response) {
	return func(request *restful.Request, response *restful.Response) {
		// authorize
		if !s.auth(request, response) {
			return
		}
		// add ratings
		feedbackLiterTime := new([]Feedback)
		if err := request.ReadEntity(feedbackLiterTime); err != nil {
			BadRequest(response, err)
			return
		}
		// parse datetime
		var err error
		feedback := make([]data.Feedback, len(*feedbackLiterTime))
		users := set.NewStringSet()
		items := set.NewStringSet()
		for i := range feedback {
			users.Add((*feedbackLiterTime)[i].UserId)
			items.Add((*feedbackLiterTime)[i].ItemId)
			feedback[i].FeedbackKey = (*feedbackLiterTime)[i].FeedbackKey
			feedback[i].Comment = (*feedbackLiterTime)[i].Comment
			feedback[i].Timestamp, err = dateparse.ParseAny((*feedbackLiterTime)[i].Timestamp)
			if err != nil {
				BadRequest(response, err)
				return
			}
		}
		// insert feedback to data store
		err = s.DataClient.BatchInsertFeedback(feedback,
			s.GorseConfig.Database.AutoInsertUser,
			s.GorseConfig.Database.AutoInsertItem, overwrite)
		if err != nil {
			InternalServerError(response, err)
			return
		}
		// insert feedback to cache store
		if err = s.InsertFeedbackToCache(feedback); err != nil {
			InternalServerError(response, err)
			return
		}

		for _, userId := range users.List() {
			err = s.CacheClient.SetTime(cache.LastModifyUserTime, userId, time.Now())
			if err != nil {
				InternalServerError(response, err)
				return
			}
		}
		for _, itemId := range items.List() {
			err = s.CacheClient.SetTime(cache.LastModifyItemTime, itemId, time.Now())
			if err != nil {
				InternalServerError(response, err)
				return
			}
		}
		Ok(response, Success{RowAffected: len(feedback)})
	}
}

// FeedbackIterator is the iterator for feedback.
type FeedbackIterator struct {
	Cursor   string
	Feedback []data.Feedback
}

func (s *RestServer) getFeedback(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Parse parameters
	cursor := request.QueryParameter("cursor")
	n, err := ParseInt(request, "n", s.GorseConfig.Server.DefaultN)
	if err != nil {
		BadRequest(response, err)
		return
	}
	cursor, feedback, err := s.DataClient.GetFeedback(cursor, n, nil)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, FeedbackIterator{Cursor: cursor, Feedback: feedback})
}

func (s *RestServer) getTypedFeedback(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Parse parameters
	feedbackType := request.PathParameter("feedback-type")
	cursor := request.QueryParameter("cursor")
	n, err := ParseInt(request, "n", s.GorseConfig.Server.DefaultN)
	if err != nil {
		BadRequest(response, err)
		return
	}
	cursor, feedback, err := s.DataClient.GetFeedback(cursor, n, nil, feedbackType)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, FeedbackIterator{Cursor: cursor, Feedback: feedback})
}

func (s *RestServer) getUserItemFeedback(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Parse parameters
	userId := request.PathParameter("user-id")
	itemId := request.PathParameter("item-id")
	if feedback, err := s.DataClient.GetUserItemFeedback(userId, itemId); err != nil {
		InternalServerError(response, err)
	} else {
		Ok(response, feedback)
	}
}

func (s *RestServer) deleteUserItemFeedback(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Parse parameters
	userId := request.PathParameter("user-id")
	itemId := request.PathParameter("item-id")
	if deleteCount, err := s.DataClient.DeleteUserItemFeedback(userId, itemId); err != nil {
		InternalServerError(response, err)
	} else {
		Ok(response, Success{RowAffected: deleteCount})
	}
}

func (s *RestServer) getTypedUserItemFeedback(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Parse parameters
	feedbackType := request.PathParameter("feedback-type")
	userId := request.PathParameter("user-id")
	itemId := request.PathParameter("item-id")
	if feedback, err := s.DataClient.GetUserItemFeedback(userId, itemId, feedbackType); err != nil {
		InternalServerError(response, err)
	} else if feedbackType == "" {
		Text(response, "{}")
	} else {
		Ok(response, feedback[0])
	}
}

func (s *RestServer) deleteTypedUserItemFeedback(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Parse parameters
	feedbackType := request.PathParameter("feedback-type")
	userId := request.PathParameter("user-id")
	itemId := request.PathParameter("item-id")
	if deleteCount, err := s.DataClient.DeleteUserItemFeedback(userId, itemId, feedbackType); err != nil {
		InternalServerError(response, err)
	} else {
		Ok(response, Success{deleteCount})
	}
}

func (s *RestServer) getMeasurements(request *restful.Request, response *restful.Response) {
	// Authorize
	if !s.auth(request, response) {
		return
	}
	// Parse parameters
	name := request.PathParameter("name")
	n, err := ParseInt(request, "n", 100)
	if err != nil {
		BadRequest(response, err)
		return
	}
	measurements, err := s.DataClient.GetMeasurements(name, n)
	if err != nil {
		InternalServerError(response, err)
		return
	}
	Ok(response, measurements)
}

// BadRequest returns a bad request error.
func BadRequest(response *restful.Response, err error) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	base.Logger().Error("bad request", zap.Error(err))
	if err = response.WriteError(http.StatusBadRequest, err); err != nil {
		base.Logger().Error("failed to write error", zap.Error(err))
	}
}

// InternalServerError returns a internal server error.
func InternalServerError(response *restful.Response, err error) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	base.Logger().Error("internal server error", zap.Error(err))
	if err = response.WriteError(http.StatusInternalServerError, err); err != nil {
		base.Logger().Error("failed to write error", zap.Error(err))
	}
}

// PageNotFound returns a not found error.
func PageNotFound(response *restful.Response, err error) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	if err := response.WriteError(http.StatusNotFound, err); err != nil {
		base.Logger().Error("failed to write error", zap.Error(err))
	}
}

// Ok sends the content as JSON to the client.
func Ok(response *restful.Response, content interface{}) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	if err := response.WriteAsJson(content); err != nil {
		base.Logger().Error("failed to write json", zap.Error(err))
	}
}

// Text returns a plain text.
func Text(response *restful.Response, content string) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	if _, err := response.Write([]byte(content)); err != nil {
		base.Logger().Error("failed to write text", zap.Error(err))
	}
}

func (s *RestServer) auth(request *restful.Request, response *restful.Response) bool {
	if s.IsDashboard || s.GorseConfig.Server.APIKey == "" {
		return true
	}
	apikey := request.HeaderParameter("X-API-Key")
	if apikey == s.GorseConfig.Server.APIKey {
		return true
	}
	base.Logger().Error("unauthorized",
		zap.String("api_key", s.GorseConfig.Server.APIKey),
		zap.String("X-API-Key", apikey))
	if err := response.WriteError(http.StatusUnauthorized, fmt.Errorf("unauthorized")); err != nil {
		base.Logger().Error("failed to write error", zap.Error(err))
	}
	return false
}

// InsertFeedbackToCache inserts feedback to cache.
func (s *RestServer) InsertFeedbackToCache(feedback []data.Feedback) error {
	for _, v := range feedback {
		err := s.CacheClient.AppendScores(cache.IgnoreItems, v.UserId, cache.Scored{v.ItemId, float32(v.Timestamp.Unix())})
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
