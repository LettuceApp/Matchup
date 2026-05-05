import React, { useState, useEffect, useRef } from 'react';
import { getUser, updateUserAvatar } from '../services/api';

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
 * - size: number, width/height of the avatar in pixels (defaults to 80) test
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
        const user = res.data?.user || res.data?.response || res.data;
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

    try {
      const uploadRes = await updateUserAvatar(userId, file);
      const updatedUser = uploadRes?.data?.user || uploadRes?.data?.response || uploadRes?.data;
      if (updatedUser?.avatar_path) {
        setAvatarKey(updatedUser.avatar_path);
      } else {
        // Re-fetch if the response didn't include avatar_path
        const res = await getUser(userId);
        const user = res.data?.user || res.data?.response || res.data;
        setAvatarKey(user.avatar_path);
      }
    } catch (err) {
      console.error('Upload error:', err);
      // Surface the underlying message (e.g. "File is too large (max 0.5 MB).")
      // when it came from our presign flow; fall back to a generic string
      // for network / RPC errors the user can't act on.
      setError(typeof err?.message === 'string' && err.message.startsWith('File is too large')
        ? err.message
        : 'Failed to upload avatar.');
    }
  };

  // Pick the smallest resized variant that still looks crisp at the
  // requested display size. Variants are uploaded by the backend's
  // resizeAndUpload helper at 100w/300w/full. Multiply by 2 for HiDPI/
  // retina displays before bucketing — a 60px avatar on a retina screen
  // wants ~120w of source, which lands in the medium bucket.
  const variantForSize = (px) => {
    const needed = px * 2;
    if (needed <= 100) return 'thumb';
    if (needed <= 300) return 'medium';
    return ''; // full original
  };

  // withImageSize inserts the variant suffix before the extension. Blob
  // preview URLs from URL.createObjectURL never match the regex so they
  // fall through unchanged.
  const withImageSize = (url, sz) => {
    if (!url || !sz) return url;
    return url.replace(/\.(jpe?g|png|gif|webp)$/i, `_${sz}.jpg`);
  };

  // Helper: if we only have the S3 key, build the full URL, then swap in
  // the resized variant matching the requested display size.
  const getSrc = (keyOrUrl) => {
    if (!keyOrUrl) return null;
    let full;
    // if it already looks like a full URL (preview or absolute), just use it
    if (keyOrUrl.startsWith('http') || keyOrUrl.startsWith('blob:')) full = keyOrUrl;
    else full = `${S3_BASE_URL}/${keyOrUrl}`;
    return withImageSize(full, variantForSize(size));
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
            loading="lazy"
            decoding="async"
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
