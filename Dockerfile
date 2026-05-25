# 1. Start with a clean, official Python environment
FROM python:3.11-slim

# 2. Create a folder named 'code' inside the container to work in
WORKDIR /code

# 3. Copy your package list file into that folder
COPY ./requirements.txt /code/requirements.txt

# 4. Run the installer to download all those Python packages
RUN pip install --no-cache-dir --upgrade -r /code/requirements.txt

# 5. Copy your todo.py code file into the container folder
COPY ./todo.py /code/todo.py

# 6. Open up port 8000 so we can access the API from outside
EXPOSE 8000

# 7. Start the server, telling it to run your 'todo.py' file
CMD ["uvicorn", "todo:app", "--host", "0.0.0.0", "--port", "8000"]