import React, { useState, useEffect, useRef } from 'react';
import API, { getUser } from '../services/api';

// —– Add these at the top —–
const S3_BUCKET   = process.env.REACT_APP_S3_BUCKET;
const AWS_REGION  = process.env.REACT_APP_AWS_REGION;
const S3_BASE_URL =
  process.env.REACT_APP_S3_BASE ||
  `https://${S3_BUCKET}.s3.${AWS_REGION}.amazonaws.com`;
// —————————————————

/**
 * ProfilePic component
 * Props:
 * - userId: ID of the user whose avatar to display
 * - editable: boolean, if true clicking the avatar opens file picker & uploads
 * - size: number, width/height of the avatar in pixels (defaults to 80)
 */
const ProfilePic = ({ userId, editable = false, size = 80 }) => {
  const [avatarKey, setAvatarKey] = useState(null);  // change name to key
  const [error, setError]       = useState('');
  const inputRef                = useRef();

  // Fetch avatar *key* from API on mount or when userId changes
  useEffect(() => {
    (async () => {
      try {
        const res  = await getUser(userId);
        const user = res.data.response || res.data;
        setAvatarKey(user.avatar_path || null);
      } catch (err) {
        console.error('Error fetching user:', err);
      }
    })();
  }, [userId]);

  const handleFileChange = async (e) => {
    const file = e.target.files[0];
    if (!file) return;
    setError('');

    // Preview locally
    const preview = URL.createObjectURL(file);
    setAvatarKey(preview);  // temporarily treat preview as a full URL

    // Upload to server
    const formData = new FormData();
    formData.append('file', file);

    try {
      await API.put(`/users/${userId}/avatar`, formData);
      // Re-fetch the real key
      const res  = await getUser(userId);
      const user = res.data.response || res.data;
      setAvatarKey(user.avatar_path);
    } catch (err) {
      console.error('Upload error:', err);
      setError('Failed to upload avatar.');
    }
  };

  // Helper: if we only have the S3 key, build the full URL
  const getSrc = (keyOrUrl) => {
    if (!keyOrUrl) return null;
    // if it already looks like a full URL (preview or absolute), just use it
    if (keyOrUrl.startsWith('http')) return keyOrUrl;
    // otherwise, prefix the bucket URL
    return `${S3_BASE_URL}/${keyOrUrl}`;
  };

  const sizePx = `${size}px`;
  const src    = getSrc(avatarKey);

  return (
    <div style={{ display: 'inline-block', textAlign: 'center' }}>
      {editable && (
        <input
          ref={inputRef}
          type="file"
          accept="image/*"
          style={{ display: 'none' }}
          onChange={handleFileChange}
        />
      )}
      <div
        onClick={() => editable && inputRef.current.click()}
        style={{
          width: sizePx,
          height: sizePx,
          borderRadius: '50%',
          overflow: 'hidden',
          cursor: editable ? 'pointer' : 'default',
          background: '#f0f0f0',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          border: src ? 'none' : '1px solid #ccc'
        }}
        title={editable ? 'Click to change avatar' : ''}
      >
        {src ? (
          <img
            src={src}
            alt="Profile"
            style={{ width: '100%', height: '100%', objectFit: 'cover' }}
          />
        ) : (
          <span style={{ fontSize: size / 5, color: '#666' }}>Profile</span>
        )}
      </div>
      {error && <p style={{ color: 'red', marginTop: 4 }}>{error}</p>}
    </div>
  );
};

export default ProfilePic;
