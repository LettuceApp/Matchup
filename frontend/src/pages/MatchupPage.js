import React, { useState, useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import NavigationBar from '../components/NavigationBar';
import MatchupItem from '../components/MatchupItem';
import Comment from '../components/Comment';
import Button from '../components/Button';
import { 
  getUserMatchup, 
  likeMatchup, 
  unlikeMatchup, 
  getUserLikes, 
  updateMatchupItem, 
  createComment, 
  getComments, 
  deleteMatchup 
} from '../services/api';

const MatchupPage = () => {
  const { uid, id } = useParams();
  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');
  const [matchup, setMatchup] = useState(null);
  const [likesCount, setLikesCount] = useState(0);
  const [isLiked, setIsLiked] = useState(false);
  const [comments, setComments] = useState([]);
  const [newComment, setNewComment] = useState('');
  const [isEditing, setIsEditing] = useState(false);
  const [editedItem, setEditedItem] = useState({});

  useEffect(() => {
    const fetchMatchup = async () => {
      try {
        const response = await getUserMatchup(uid, id);
        setMatchup(response.data.response);

        const userLikesResponse = await getUserLikes(userId);
        const likedMatchup = userLikesResponse.data.response.some(
          like => like.matchup_id === parseInt(id)
        );
        setIsLiked(likedMatchup);
        setLikesCount(response.data.response.likes_count);

        const commentsResponse = await getComments(id);
        setComments(commentsResponse.data.response);
      } catch (err) {
        console.error('Failed to fetch matchup:', err);
      }
    };

    fetchMatchup();
  }, [uid, id, userId]);

  const handleLikeToggle = async () => {
    try {
      if (isLiked) {
        await unlikeMatchup(id);
        setLikesCount(likesCount - 1);
      } else {
        await likeMatchup(id);
        setLikesCount(likesCount + 1);
      }
      setIsLiked(!isLiked);
    } catch (err) {
      console.error('Failed to toggle like on matchup:', err);
    }
  };

  const refreshItems = async () => {
    try {
      const response = await getUserMatchup(uid, id);
      setMatchup(response.data.response);

      const commentsResponse = await getComments(id);
      setComments(commentsResponse.data.response);
    } catch (err) {
      console.error('Failed to refresh items:', err);
    }
  };

  const handleSave = async () => {
    try {
      await updateMatchupItem(editedItem.id, { item: editedItem.item });
      setIsEditing(false);
      refreshItems();
    } catch (err) {
      console.error('Failed to save item:', err);
    }
  };

  const handleCancel = () => {
    setIsEditing(false);
    refreshItems();
  };

  const handleEdit = (item) => {
    setEditedItem(item);
    setIsEditing(true);
  };

  const handleCommentSubmit = async (e) => {
    e.preventDefault();
    try {
      await createComment(id, { Body: newComment });
      setNewComment('');
      refreshItems();
    } catch (err) {
      console.error('Failed to add comment:', err);
    }
  };

  // New function to handle deletion of the matchup
  const handleDelete = async () => {
    try {
      await deleteMatchup(id);
      // Redirect back to the homepage once deleted
      navigate('/');
    } catch (err) {
      console.error('Failed to delete matchup:', err);
    }
  };

  if (!matchup) return <p>Loading matchup...</p>;

  // Only allow deletion if the logged-in user is the owner of the matchup
  const isOwner = matchup.author_id === parseInt(userId);

  return (
    <div>
      <NavigationBar />
      <h1>{matchup.title}</h1>
      <p><strong>Description:</strong> {matchup.content}</p>
      <p>Likes: {likesCount}</p>
      <button onClick={handleLikeToggle}>
        {isLiked ? 'Unlike' : 'Like'}
      </button>
      
      {/* Render Delete button only for the owner */}
      {isOwner && (
        <Button onClick={handleDelete}>
          Delete Matchup
        </Button>
      )}
      
      <div>
        <h2>Items and Scores</h2>
        {matchup.items.map((item) => (
          <MatchupItem
            key={item.id}
            item={item}
            isOwner={isOwner}
            refreshItems={refreshItems}
            isEditing={isEditing}
            setIsEditing={setIsEditing}
            handleEdit={handleEdit}
          />
        ))}
        {isOwner && isEditing && (
          <div>
            <Button onClick={handleSave}>Save</Button>
            <Button onClick={handleCancel}>Cancel</Button>
          </div>
        )}
      </div>
      <div>
        <h2>Comments</h2>
        {comments.length > 0 ? (
          comments.map((comment) => (
            <Comment key={comment.id} comment={comment} refreshComments={refreshItems} />
          ))
        ) : (
          <p>No comments available.</p>
        )}
        <form onSubmit={handleCommentSubmit}>
          <div>
            <textarea
              value={newComment}
              onChange={(e) => setNewComment(e.target.value)}
              placeholder="Add a comment"
              required
              style={{ width: '50%', height: '50px', display: 'block' }}
            />
          </div>
          <div style={{ marginTop: '10px' }}>
            <Button type="submit">Submit</Button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default MatchupPage;
