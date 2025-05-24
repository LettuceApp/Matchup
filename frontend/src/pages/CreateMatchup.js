import React, { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import NavigationBar from '../components/NavigationBar';
import Button from '../components/Button';
import { createMatchup } from '../services/api';

const CreateMatchup = () => {
  // Extract the user id from the route params (the route is set as /users/:id/create-matchup)
  const { userId } = useParams();
  const navigate = useNavigate();

  // State for the form fields
  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');
  const [items, setItems] = useState([{ item: '' }]);

  // Handle changes for the title and content fields
  const handleTitleChange = (e) => setTitle(e.target.value);
  const handleContentChange = (e) => setContent(e.target.value);

  // Handle changes in each item input field
  const handleItemChange = (index, value) => {
    const newItems = [...items];
    newItems[index].item = value;
    setItems(newItems);
  };

  // Add a new empty item field
  const addItem = () => {
    setItems([...items, { item: '' }]);
  };

  // Remove an item field (if more than one exists)
  const removeItem = (index) => {
    if (items.length > 1) {
      const newItems = items.filter((_, i) => i !== index);
      setItems(newItems);
    }
  };

  // Handle the form submission
  const handleSubmit = async (e) => {
    e.preventDefault();

    // Create the JSON data object
    const matchupData = {
      title,
      content,
      items,
    };

    try {
      // Call the API function, which will post to /users/:id/create-matchup
      const response = await createMatchup(userId, matchupData);
      console.log('Matchup created:', response.data);

      // Retrieve the newly created matchup's id from the response.
      // (Assuming your backend sends it as response.data.response.id)
      const newMatchupId = response.data.response.id;
      
      
      // Redirect to the matchup detail page using the new id
      navigate(`/users/${userId}/matchup/${newMatchupId}`);
    } catch (error) {
      console.error('Error creating matchup:', error);
    }
  };

  return (
    <div>
      <NavigationBar />
      <h1>Create a New Matchup</h1>
      <form onSubmit={handleSubmit}>
        <div>
          <label>Title:</label>
          <input 
            type="text" 
            value={title} 
            onChange={handleTitleChange}
            required
          />
        </div>
        <div>
          <label>Content:</label>
          <textarea 
            value={content} 
            onChange={handleContentChange}
            required
          />
        </div>
        <div>
          <label>Items:</label>
          {items.map((itm, index) => (
            <div key={index}>
              <input 
                type="text" 
                value={itm.item} 
                onChange={(e) => handleItemChange(index, e.target.value)}
                placeholder={`Item ${index + 1}`}
                required
              />
              {items.length > 1 && (
                <Button type="button" onClick={() => removeItem(index)}>
                  Remove
                </Button>
              )}
            </div>
          ))}
          <Button type="button" onClick={addItem}>
            Add Item
          </Button>
        </div>
        <Button type="submit">Create Matchup</Button>
      </form>
    </div>
  );
};

export default CreateMatchup;
