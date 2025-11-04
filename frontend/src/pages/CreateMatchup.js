import React, { useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import NavigationBar from '../components/NavigationBar';
import Button from '../components/Button';
import { createMatchup } from '../services/api';
import '../styles/CreateMatchup.css';

const CreateMatchup = () => {
  const { userId } = useParams();
  const navigate = useNavigate();

  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');
  const [items, setItems] = useState([{ item: '' }, { item: '' }]);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState(null);

  const handleTitleChange = (e) => setTitle(e.target.value);
  const handleContentChange = (e) => setContent(e.target.value);

  const handleItemChange = (index, value) => {
    const newItems = [...items];
    newItems[index].item = value;
    setItems(newItems);
  };

  const addItem = () => {
    setItems([...items, { item: '' }]);
  };

  const removeItem = (index) => {
    if (items.length > 1) {
      const newItems = items.filter((_, i) => i !== index);
      setItems(newItems);
    }
  };

  const goBack = () => navigate(-1);

  const sanitizedItems = useMemo(
    () => items.map(({ item }) => ({ item: item.trim() })),
    [items]
  );

  const filledItems = useMemo(
    () => sanitizedItems.filter(({ item }) => item.length > 0),
    [sanitizedItems]
  );

  const hasRequiredContenders = filledItems.length >= 2;

  const isCreateDisabled =
    isSubmitting ||
    title.trim().length === 0 ||
    content.trim().length === 0 ||
    filledItems.length !== items.length ||
    !hasRequiredContenders;

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (isCreateDisabled) {
      return;
    }

    const matchupData = {
      title: title.trim(),
      content: content.trim(),
      items: sanitizedItems,
    };

    try {
      setIsSubmitting(true);
      setError(null);
      const response = await createMatchup(userId, matchupData);
      console.log('Matchup created:', response.data);

      const newMatchupId = response.data.response.id;

      navigate(`/users/${userId}/matchup/${newMatchupId}`);
    } catch (error) {
      console.error('Error creating matchup:', error);
      setError('We could not create that matchup. Please review your entries and try again.');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="create-matchup-page">
      <NavigationBar />
      <main className="create-matchup-content">
        <section className="create-matchup-hero">
          <p className="create-overline">New Matchup</p>
          <h1>Bring a fresh head-to-head to the community.</h1>
          <p className="create-subtitle">
            Set the stage with a captivating title, tell everyone what the clash is about, and add contenders to get the debate started.
          </p>
          <div className="create-hero-actions">
            <Button onClick={() => navigate('/')} className="create-secondary-button">
              Back to dashboard
            </Button>
            <Button onClick={goBack} className="create-tertiary-button">
              Go back
            </Button>
          </div>
        </section>

        <section className="create-layout">
          <form onSubmit={handleSubmit} className="create-form">
            <div className="create-form-group">
              <label htmlFor="matchup-title">
                Matchup title <span aria-hidden="true">*</span>
              </label>
              <input
                id="matchup-title"
                type="text"
                value={title}
                onChange={handleTitleChange}
                placeholder="e.g. Lakers vs Warriors: Who takes the series?"
                className="create-input"
                required
              />
            </div>

            <div className="create-form-group">
              <label htmlFor="matchup-content">
                Description <span aria-hidden="true">*</span>
              </label>
              <textarea
                id="matchup-content"
                value={content}
                onChange={handleContentChange}
                placeholder="Give everyone the context and criteria for your matchup."
                className="create-textarea"
                rows={6}
                required
              />
            </div>

            <div className="create-form-group">
              <div className="create-form-group-header">
                <label>Contenders</label>
                <span className="create-form-hint">Add at least two contenders to keep things interesting.</span>
              </div>

              <div className="create-items">
                {items.map((itm, index) => (
                  <div key={index} className="create-item-row">
                    <input
                      type="text"
                      value={itm.item}
                      onChange={(e) => handleItemChange(index, e.target.value)}
                      placeholder={`Contender ${index + 1}`}
                      className="create-input"
                      required
                    />
                    {items.length > 1 && (
                      <Button
                        type="button"
                        onClick={() => removeItem(index)}
                        className="create-remove-button"
                      >
                        Remove
                      </Button>
                    )}
                  </div>
                ))}
              </div>

              <Button type="button" onClick={addItem} className="create-add-button">
                + Add another contender
              </Button>
              {!hasRequiredContenders && (
                <p className="create-inline-hint" role="alert">
                  You need at least two contenders to create a matchup.
                </p>
              )}
            </div>

            {error && <p className="create-error">{error}</p>}

            <div className="create-form-actions">
              <Button
                type="submit"
                className="create-primary-button"
                disabled={isCreateDisabled}
              >
                {isSubmitting ? 'Creating...' : 'Create matchup'}
              </Button>
              <Button type="button" onClick={goBack} className="create-tertiary-button">
                Cancel
              </Button>
            </div>
          </form>

          <aside className="create-preview-card">
            <div className="create-preview-header">
              <span className="create-preview-label">Live Preview</span>
              <span className="create-preview-count">{filledItems.length} contenders</span>
            </div>
            <div className="create-preview-body">
              <h2>{title.trim() || 'Your matchup title will appear here'}</h2>
              <p>{content.trim() || 'Use the description field to explain what makes this matchup exciting.'}</p>
              <ul className="create-preview-list">
                {(filledItems.length > 0 ? filledItems : sanitizedItems).map(({ item }, index) => (
                  <li key={index}>
                    <span className="create-preview-bullet" aria-hidden="true" />
                    <span>{item.length > 0 ? item : `Contender ${index + 1}`}</span>
                  </li>
                ))}
              </ul>
            </div>
          </aside>
        </section>
      </main>
    </div>
  );
};

export default CreateMatchup;
