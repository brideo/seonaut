package api

import "net/http"

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
}

// createUser handles POST /api/users.
func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(w, r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, codeBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	user, err := h.Users.SignUp(req.Email, req.Password, defaultLang, defaultTheme)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, userResponse{ID: user.Id, Email: user.Email})
}
