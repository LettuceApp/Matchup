// PostedComment.js
import React from 'react';
import { Link } from 'react-router-dom';

const PostedComment = ({ comment }) => {
  const { commenterName, commenterId, body, createdAt } = comment;

  return (
    <div style={{ marginBottom: '10px', padding: '10px', borderBottom: '1px solid #ddd' }}>
      {/* Commenter's Name with Link to Profile */}
      <div>
        <Link to={`/users/${commenterId}/profile`} style={{ fontWeight: 'bold', textDecoration: 'none', color: '#3b5998' }}>
          {commenterName}
        </Link>
        <span style={{ marginLeft: '10px', color: '#555', fontSize: '12px' }}>
          {new Date(createdAt).toLocaleString()}
        </span>
      </div>

      {/* Comment Body */}
      <div style={{ marginTop: '5px', color: '#333' }}>
        {body}
      </div>
    </div>
  );
};

export default PostedComment;