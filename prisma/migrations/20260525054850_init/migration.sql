-- DropForeignKey
ALTER TABLE "Exercise" DROP CONSTRAINT "Exercise_workoutId_fkey";

-- DropForeignKey
ALTER TABLE "ExerciseSession" DROP CONSTRAINT "ExerciseSession_exerciseId_fkey";

-- DropForeignKey
ALTER TABLE "ExerciseSession" DROP CONSTRAINT "ExerciseSession_workoutSessionId_fkey";

-- DropForeignKey
ALTER TABLE "SetSession" DROP CONSTRAINT "SetSession_exerciseSessionId_fkey";

-- DropForeignKey
ALTER TABLE "Workout" DROP CONSTRAINT "Workout_userId_fkey";

-- DropForeignKey
ALTER TABLE "WorkoutSession" DROP CONSTRAINT "WorkoutSession_userId_fkey";

-- DropForeignKey
ALTER TABLE "WorkoutSession" DROP CONSTRAINT "WorkoutSession_workoutId_fkey";

-- AddForeignKey
ALTER TABLE "Workout" ADD CONSTRAINT "Workout_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Exercise" ADD CONSTRAINT "Exercise_workoutId_fkey" FOREIGN KEY ("workoutId") REFERENCES "Workout"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "WorkoutSession" ADD CONSTRAINT "WorkoutSession_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "WorkoutSession" ADD CONSTRAINT "WorkoutSession_workoutId_fkey" FOREIGN KEY ("workoutId") REFERENCES "Workout"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "ExerciseSession" ADD CONSTRAINT "ExerciseSession_workoutSessionId_fkey" FOREIGN KEY ("workoutSessionId") REFERENCES "WorkoutSession"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "ExerciseSession" ADD CONSTRAINT "ExerciseSession_exerciseId_fkey" FOREIGN KEY ("exerciseId") REFERENCES "Exercise"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "SetSession" ADD CONSTRAINT "SetSession_exerciseSessionId_fkey" FOREIGN KEY ("exerciseSessionId") REFERENCES "ExerciseSession"("id") ON DELETE CASCADE ON UPDATE CASCADE;
