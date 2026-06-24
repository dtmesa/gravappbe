-- DropIndex
DROP INDEX "Exercise_workoutId_idx";

-- DropIndex
DROP INDEX "ExerciseSession_exerciseId_workoutSessionId_idx";

-- DropIndex
DROP INDEX "SetSession_exerciseSessionId_idx";

-- DropIndex
DROP INDEX "WorkoutSession_userId_workoutId_date_idx";

-- CreateIndex
CREATE INDEX "ExerciseSession_workoutSessionId_idx" ON "ExerciseSession"("workoutSessionId");

-- CreateIndex
CREATE INDEX "ExerciseSession_exerciseId_idx" ON "ExerciseSession"("exerciseId");

-- CreateIndex
CREATE INDEX "SetSession_exerciseSessionId_order_idx" ON "SetSession"("exerciseSessionId", "order");

-- CreateIndex
CREATE INDEX "Workout_userId_idx" ON "Workout"("userId");

-- CreateIndex
CREATE INDEX "WorkoutSession_userId_idx" ON "WorkoutSession"("userId");

-- CreateIndex
CREATE INDEX "WorkoutSession_workoutId_idx" ON "WorkoutSession"("workoutId");

-- CreateIndex
CREATE INDEX "WorkoutSession_userId_workoutId_idx" ON "WorkoutSession"("userId", "workoutId");

-- CreateIndex
CREATE INDEX "WorkoutSession_workoutId_date_idx" ON "WorkoutSession"("workoutId", "date");
