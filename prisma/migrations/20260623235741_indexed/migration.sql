-- CreateIndex
CREATE INDEX "Exercise_workoutId_idx" ON "Exercise"("workoutId");

-- CreateIndex
CREATE INDEX "Exercise_workoutId_order_idx" ON "Exercise"("workoutId", "order");

-- CreateIndex
CREATE INDEX "ExerciseSession_exerciseId_workoutSessionId_idx" ON "ExerciseSession"("exerciseId", "workoutSessionId");

-- CreateIndex
CREATE INDEX "SetSession_exerciseSessionId_idx" ON "SetSession"("exerciseSessionId");

-- CreateIndex
CREATE INDEX "Workout_userId_order_idx" ON "Workout"("userId", "order");

-- CreateIndex
CREATE INDEX "WorkoutSession_userId_workoutId_date_idx" ON "WorkoutSession"("userId", "workoutId", "date");
