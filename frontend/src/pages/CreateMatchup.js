import React, { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import NavigationBar from '../components/NavigationBar';
import MatchupItem from '../components/MatchupItem';
import PostedComment from '../components/PostedComment'; // Import PostedComment component
import Button from '../components/Button';
import { getUserMatchup, likeMatchup, unlikeMatchup, getUserLikes, updateMatchupItem, createComment, getComments } from '../services/api';

const MatchupPage = () => {
  const { uid, id } = useParams(); // Use uid and id from the URL parameters
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
        const response = await getUserMatchup(uid, id); // Use uid to fetch the matchup for the correct owner
        setMatchup(response.data.response);

        const userLikesResponse = await getUserLikes(userId);
        const likedMatchup = userLikesResponse.data.response.some(like => like.matchup_id === parseInt(id));
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
      const response = await getUserMatchup(uid, id); // Refresh the matchup using the correct uid
      setMatchup(response.data.response);

      const commentsResponse = await getComments(id);
      setComments(commentsResponse.data.response);
    } catch (err) {
      console.error('Failed to refresh items:', err);
    }
  };

  const handleSave = async () => {
    try {
      console.log('Request Body:', JSON.stringify({ item: editedItem.item }));
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

  if (!matchup) return <p>Loading matchup...</p>;

  const isOwner = matchup.author_id === parseInt(userId);

  return (
    <div>
      <NavigationBar />
      <h1>{matchup.title}</h1>
      <p><strong>Description:</strong> {matchup.content}</p>
      <p>Likes: {likesCount}</p>
      <button onClick={handleLikeToggle}>{isLiked ? 'Unlike' : 'Like'}</button>
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
        
        {/* Display PostedComment for each comment */}
        {comments.length > 0 ? (
          comments.map((comment) => (
            <PostedComment key={comment.id} comment={comment} />
          ))
        ) : (
          <p>No comments available.</p>
        )}
        
        {/* Comment Input Form */}
        <form onSubmit={handleCommentSubmit}>
          <textarea
            value={newComment}
            onChange={(e) => setNewComment(e.target.value)}
            placeholder="Add a comment"
            required
          />
          <Button type="submit" onClick={() => {}}>Submit</Button>
        </form>
      </div>
    </div>
  );
};

export default MatchupPage;